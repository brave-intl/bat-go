package redisconsumer

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/concurrent"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	redis "github.com/redis/go-redis/v9"
)

// RetryAfterPrefix is the redis key prefix for storing the next advised retry time for a message
const RetryAfterPrefix = "retry-after-"

// ConsumerSet tracks currently running in-process redis stream consumers to prevent duplicates
var ConsumerSet *concurrent.Set

func init() {
	ConsumerSet = concurrent.NewSet()
}

// MessageHandler is a function for handling stream messages
type MessageHandler func(ctx context.Context, id string, data []byte) error

// StreamClient is the generic type inferface for a client of redis streams
type StreamClient interface {
	// CreateStream if it does not already exist
	CreateStream(ctx context.Context, stream, consumerGroup string) error
	// AddMessges to the specified stream
	AddMessages(ctx context.Context, stream string, message ...interface{}) error
	// ReadAndHandle any messages for the specified consumer, including any retries
	ReadAndHandle(ctx context.Context, stream, consumerGroup, consumerID string, handle MessageHandler)
	// UnacknowledgedCount returns the count of messages which are either unread or pending
	UnacknowledgedCounts(ctx context.Context, stream, consumerGroup string) (lag int64, pending int64, err error)
	// GetMessageRetryAfter returning true if a retry-after existing for message id
	GetMessageRetryAfter(ctx context.Context, id string) (bool, error)
	// SetMessageRetryAfter for message id, expiring after delay
	SetMessageRetryAfter(ctx context.Context, id string, delay time.Duration) error
}

// RedisClient is an implementation of StreamClient using an actual redis connection
type RedisClient redis.Client

func NewStreamClient(redisClient *redis.Client) *RedisClient {
	return (*RedisClient)(redisClient)
}

// CreateStream if it does not already exist
func (redisClient *RedisClient) CreateStream(ctx context.Context, stream, consumerGroup string) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}

	err = redisClient.XGroupCreateMkStream(ctx, stream, consumerGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		logger.Error().Err(err).Msg("XGROUP CREATE MKSTREAM failed")
		return err
	}
	return nil
}

// AddMessage to the specified redis stream
func (redisClient *RedisClient) AddMessages(ctx context.Context, stream string, message ...interface{}) error {
	pipe := ((*redis.Client)(redisClient)).Pipeline()

	for _, v := range message {
		pipe.XAdd(
			ctx, &redis.XAddArgs{
				Stream: stream,
				Values: map[string]interface{}{
					"data": v}},
		)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// ReadAndHandle any messages for the specified consumer, including any retries
func (redisClient *RedisClient) ReadAndHandle(ctx context.Context, stream, consumerGroup, consumerID string, handle MessageHandler) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return
	}

	readAndHandle := func(id string) int {
		entries, err := redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: consumerID,
			Streams:  []string{stream, id},
			Count:    5,
			Block:    100 * time.Millisecond,
			NoAck:    false,
		}).Result()
		if err != nil && !strings.Contains(err.Error(), "redis: nil") {
			logger.Error().Err(err).Msg("XREADGROUP failed")
		}

		if len(entries) > 0 {
			for i := 0; i < len(entries[0].Messages); i++ {
				messageID := entries[0].Messages[i].ID
				values := entries[0].Messages[i].Values
				data, exists := values["data"]
				if !exists {
					logger.Error().Msg("data did not exist in message")
				}
				sData, ok := data.(string)
				if !ok {
					logger.Error().Msg("data was not a string")
				}

				tmp := logger.With().Str("messageID", messageID).Logger()
				logger = &tmp
				ctx = logger.WithContext(ctx)

				err := handle(ctx, messageID, []byte(sData))
				if err != nil {
					if !strings.Contains(err.Error(), "retry-after") {
						logger.Warn().Err(err).Msg("message handler returned an error")
					}
				} else {
					redisClient.XAck(ctx, stream, consumerGroup, messageID)
				}
			}
		}

		return len(entries)
	}
	// first read and handle new messages
	n := readAndHandle(">")
	if n == 0 {
		// then read and handle pending messages once there are no more new messages
		readAndHandle("0")
	}
}

// UnacknowledgedCounts returns the count of messages which are either unread or pending
// NOTE: this is only accurate if no messages were deleted
func (redisClient *RedisClient) UnacknowledgedCounts(ctx context.Context, stream, consumerGroup string) (lag int64, pending int64, err error) {
	resp, err := redisClient.XInfoGroups(ctx, stream).Result()
	if err != nil {
		return -1, -1, err
	}
	for _, group := range resp {
		if group.Name == consumerGroup {
			return group.Lag, group.Pending, nil
		}
	}
	return -1, -1, errors.New("unable to find specified group")
}

// GetMessageRetryAfter returning true if a retry-after existing for message id
func (redisClient *RedisClient) GetMessageRetryAfter(ctx context.Context, id string) (bool, error) {
	err := redisClient.Get(ctx, RetryAfterPrefix+id).Err()
	if err == nil {
		return true, nil
	}
	if err != redis.Nil {
		return false, err
	}

	return false, nil
}

// SetMessageRetryAfter for message id, expiring after delay
func (redisClient *RedisClient) SetMessageRetryAfter(ctx context.Context, id string, delay time.Duration) error {
	return redisClient.Set(ctx, RetryAfterPrefix+id, "", delay).Err()
}

// StartConsumer using a generic stream client
// NOTE: control will remain in this function until context cancellation
func StartConsumer(ctx context.Context, streamClient StreamClient, stream, consumerGroup, consumerID string, handle MessageHandler) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}

	consumerUID := stream + consumerGroup + consumerID
	if !ConsumerSet.Add(consumerUID) {
		// Another identical consumer is already running
		return nil
	}
	if err != nil {
		return err
	}
	ctx, logger = logging.UpdateContext(ctx, logger.With().Str("stream", stream).Str("consumerGroup", consumerGroup).Str("consumerID", consumerID).Logger())

	logger.Info().Msg("consumer started")

	streamClient.CreateStream(ctx, stream, consumerGroup)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			streamClient.ReadAndHandle(ctx, stream, consumerGroup, consumerID, handle)
		case <-ctx.Done():
			logger.Info().Msg("shutting down consumer")
			return nil
		}
	}
}

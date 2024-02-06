package redisconsumer

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/brave-intl/bat-go/libs/concurrent"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/rootdir"
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
type MessageHandler func(ctx context.Context, stream, id string, data []byte) error

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
	// GetStreamLength returns the stream length
	GetStreamLength(ctx context.Context, stream string) (int64, error)
	// GetMessageRetryAfter returning true if a retry-after existing for message id
	GetMessageRetryAfter(ctx context.Context, id string) (bool, error)
	// SetMessageRetryAfter for message id, expiring after delay
	SetMessageRetryAfter(ctx context.Context, id string, delay time.Duration) error
}

// RedisClient is an implementation of StreamClient using an actual redis connection
type RedisClient redis.Client

func NewStreamClient(ctx context.Context, env, addr, user, pass string, useTLS bool) (*RedisClient, error) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return nil, err
	}

	var tlsConfig *tls.Config
	if useTLS {
		tlsConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			ClientAuth: 0,
		}
	}

	// only if environment is local do we hardcode these values
	if tlsConfig != nil && env == "local" {
		certPool := x509.NewCertPool()
		pem, err := ioutil.ReadFile(filepath.Join(rootdir.Path, "./libs/redisconsumer/tests/tls/ca.crt"))
		if err != nil {
			return nil, fmt.Errorf("failed to read test-mode ca.crt: %w", err)
		}
		certPool.AppendCertsFromPEM(pem)
		tlsConfig.RootCAs = certPool
	}
	rc := redis.NewClient(
		&redis.Options{
			Addr: addr, Password: pass, Username: user,
			DialTimeout:     15 * time.Second,
			WriteTimeout:    5 * time.Second,
			MaxRetries:      5,
			MinRetryBackoff: 5 * time.Millisecond,
			MaxRetryBackoff: 500 * time.Millisecond,
			TLSConfig:       tlsConfig,
		},
	)

	_, err = rc.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to setup redis client: %w", err)
	}
	logger.Info().Msg("ping success, redis client connected")

	return (*RedisClient)(rc), nil
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
func (redisClient *RedisClient) AddMessages(ctx context.Context, stream string, messages ...interface{}) error {
	pipe := ((*redis.Client)(redisClient)).Pipeline()
	var err error

	// to avoid errors enqueuing large message sets, enqueue them in chunks
	for _, messages := range chunkMessages(messages) {
		// loop again so that each message gets its own record
		for _, message := range messages {
			pipe.XAdd(
				ctx, &redis.XAddArgs{
					Stream: stream,
					Values: map[string]interface{}{
						"data": message}},
			)
		}
		_, err = pipe.Exec(ctx)
		if err != nil {
			return err
		}
	}
	return err
}

func chunkMessages(messages []interface{}) [][]interface{} {
	var chunks [][]interface{}
	chunkSize := 500
	for i := 0; i < len(messages); i += chunkSize {
		end := i + chunkSize
		if len(messages) < end {
			end = len(messages)
		}
		chunks = append(chunks, messages[i:end])
	}
	return chunks
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
			Count:    10,
			Block:    100 * time.Millisecond,
			NoAck:    false,
		}).Result()
		if err != nil && !strings.Contains(err.Error(), "redis: nil") {
			logger.Error().Err(err).Msg("XREADGROUP failed")
		}

		if len(entries) > 0 {
			var wg sync.WaitGroup
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

				wg.Add(1)
				go func() {
					defer wg.Done()
					err := handle(ctx, stream, messageID, []byte(sData))
					if err != nil {
						if !strings.Contains(err.Error(), "retry-after") {
							logger.Warn().Err(err).Msg("message handler returned an error")
						}
					} else {
						redisClient.XAck(ctx, stream, consumerGroup, messageID)
					}
				}()
			}
			wg.Wait()
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
	err = redisClient.CreateStream(ctx, stream, consumerGroup)
	if err != nil {
		return -1, -1, err
	}

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

// GetStreamLength returns the stream length
func (redisClient *RedisClient) GetStreamLength(ctx context.Context, stream string) (int64, error) {
	return redisClient.XLen(ctx, stream).Result()
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
	defer ConsumerSet.Remove(consumerUID)

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

package event

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/go-redis/redis/v8"
	uuid "github.com/satori/go.uuid"
)

// busyGroup when a consumer group with the same name already exists redis returns a BUSYGROUP error
const busyGroup = "BUSYGROUP"

// Default values used by batchConsumerConfig
const (
	// start is the id we begin processing at
	start = "0"
	// count is the maximum number of messages to read from the stream
	count = 10
	// block is the time.Duration a consumer should block when the stream is empty
	block = 0
	//minIdleTime is the minimum time a message can should be idle before being claimed for reprocess
	minIdleTime = 5 * time.Second
	// retryDelay is the minimum delay before polling the pending stream for non ack messages
	retryDelay = time.Minute * 1
)

type (
	batchConsumer struct {
		redis           *Client
		config          batchConsumerConfig
		handler         Handler
		router          Router
		deadLetterQueue string
	}

	batchConsumerConfig struct {
		streamName    string
		consumerID    uuid.UUID
		consumerGroup string
		start         string
		count         int64
		block         time.Duration
		minIdleTime   time.Duration
		retryDelay    time.Duration
	}

	// Handler defines a message handler
	Handler interface {
		Handle(ctx context.Context, messages []Message) error
	}

	// Router defines a router function
	Router func(message *Message) error
)

func NewBatchConsumer(redis *Client, config batchConsumerConfig, handler Handler, router Router,
	deadLetterQueue string) (*batchConsumer, error) {
	return &batchConsumer{
		redis:           redis,
		config:          config,
		handler:         handler,
		router:          router,
		deadLetterQueue: deadLetterQueue,
	}, nil
}

func (b *batchConsumer) Consume(ctx context.Context) error {
	logging.FromContext(ctx).UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Str("consumer_id", b.config.consumerID.String()).
			Str("consumer_group", b.config.consumerGroup).
			Str("stream_name", b.config.streamName)
	})

	// create new or connect to existing consumer group
	_, err := b.redis.XGroupCreateMkStream(ctx, b.config.streamName, b.config.consumerGroup, b.config.start).Result()
	if err != nil && !strings.Contains(err.Error(), busyGroup) {
		return fmt.Errorf("error creating consumer group %w", err)
	}

	go b.process(ctx)
	go b.retry(ctx)

	return nil
}

// process reads messages from the stream for processing
func (b *batchConsumer) process(ctx context.Context) {

	xReadGroupArgs := &redis.XReadGroupArgs{
		Streams:  []string{b.config.streamName, ">"},
		Group:    b.config.consumerGroup,
		Consumer: b.config.consumerID.String(),
		Count:    b.config.count,
		Block:    b.config.block,
	}

	for {

		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				logging.FromContext(ctx).Error().
					Err(fmt.Errorf("error context cancelled: %w", err)).
					Msg("batch consumer")
			}
			return
		default:
			streams, err := b.redis.XReadGroup(ctx, xReadGroupArgs).Result()
			if err != nil {
				logging.FromContext(ctx).Error().
					Err(fmt.Errorf("error reading stream: %w", err)).
					Msg("batch consumer")
			}

			// check we have messages to process
			if len(streams) < 1 {
				continue
			}

			var xMessageIDs []string
			var messages []Message

			for _, xMessage := range streams[0].Messages {

				logging.FromContext(ctx).Info().Msgf("processing message: xMessageID %s", xMessage.ID)

				d, ok := xMessage.Values[dataKey]
				if !ok {
					err := b.redis.SendRawMessage(ctx, xMessage.Values, b.deadLetterQueue)
					if err != nil {
						logging.FromContext(ctx).Error().
							Err(fmt.Errorf("error retrieving dataKey for messageID %s: %w", xMessage.ID, err)).
							Msg("batch consumer")
					}
					continue
				}

				switch s := d.(type) {
				case string:
					xMessageIDs = append(xMessageIDs, xMessage.ID)

					message, err := NewMessageFromString(s)
					if err != nil || len(message.Body) == 0 {

						logging.FromContext(ctx).Error().
							Err(fmt.Errorf("error creating new message for messageID %s: %w", xMessage.ID, err)).
							Msg("batch consumer")

						err := b.redis.SendRawMessage(ctx, xMessage.Values, b.deadLetterQueue)
						if err != nil {
							logging.FromContext(ctx).Error().
								Err(fmt.Errorf("error sending messageID %s to dlq: %w", message.ID, err)).
								Msg("batch consumer")
						}

						// skip further processing
						continue
					}

					if _, ok := message.Headers[HeaderCorrelationID]; !ok {
						message.Headers[HeaderCorrelationID] = uuid.NewV4().String()
					}

					if b.router != nil {
						err := b.router(message)
						if err != nil {

							logging.FromContext(ctx).Error().
								Err(fmt.Errorf("error adding router for messageID %s: %w", message.ID, err)).
								Msg("batch consumer")

							err := b.redis.SendRawMessage(ctx, xMessage.Values, b.deadLetterQueue)
							if err != nil {
								logging.FromContext(ctx).Error().
									Err(fmt.Errorf("error sending messageID %s to dlq: %w", message.ID, err)).
									Msg("batch consumer")
							}
							continue
						}
					}

					messages = append(messages, *message)

				default:
					logging.FromContext(ctx).Error().
						Err(fmt.Errorf("error invalid data type for %s", xMessage.ID)).
						Msg("batch consumer")

					err := b.redis.SendRawMessage(ctx, xMessage.Values, b.deadLetterQueue)
					if err != nil {
						logging.FromContext(ctx).Error().
							Err(fmt.Errorf("error sending messageID %s to dlq: %w", xMessage.ID, err)).
							Msg("batch consumer")
					}
				}
			}

			err = b.handler.Handle(ctx, messages)
			if err != nil {
				logging.FromContext(ctx).Error().
					Err(fmt.Errorf("error handling messages: %w", err)).
					Msg("error processing message")
				continue
			}

			if _, err := b.redis.XAck(ctx, b.config.streamName, b.config.consumerGroup, xMessageIDs...).Result(); err != nil {
				logging.FromContext(ctx).Error().
					Err(fmt.Errorf("error ack messages: %w", err)).
					Msg("error ack messages")
			}
		}
	}
}

// retry is responsible for consuming messaged that have failed to process first time
func (b *batchConsumer) retry(ctx context.Context) {

	xAutoClaimArgs := &redis.XAutoClaimArgs{
		Stream:   b.config.streamName,
		Group:    b.config.consumerGroup,
		Consumer: b.config.consumerID.String(),
		MinIdle:  b.config.minIdleTime,
		Start:    b.config.start,
		Count:    b.config.count,
	}

	timer := time.NewTimer(b.config.retryDelay)

	for {

		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				logging.FromContext(ctx).Error().Err(err).Msg("error reading from stream")
			}
			return
		case <-timer.C:

			xMessages, _, err := b.redis.XAutoClaim(ctx, xAutoClaimArgs).Result()
			if err != nil {
				logging.FromContext(ctx).Error().Err(err).Msg("error decoding message")
				continue
			}

			// if message has high retry send to dlq otherwise process
			for _, xMessage := range xMessages {

				d, ok := xMessage.Values[dataKey]
				if !ok {
					err := b.redis.SendRawMessage(ctx, xMessage.Values, b.deadLetterQueue)
					if err != nil {
						logging.FromContext(ctx).Error().Err(err).Msg("error decoding message")
					}
					continue
				}

				switch s := d.(type) {
				case string:

					message, err := NewMessageFromString(s)
					if err != nil {
						logging.FromContext(ctx).Error().Err(err).Msg("error decoding message")
						err := b.redis.SendRawMessage(ctx, xMessage.Values, b.deadLetterQueue)
						if err != nil {
							logging.FromContext(ctx).Error().Err(err).Msg("error decoding message")
						}
						continue
					}

					if _, ok := message.Headers[HeaderCorrelationID]; !ok {
						message.Headers[HeaderCorrelationID] = uuid.NewV4().String()
					}

					err = b.handler.Handle(ctx, []Message{*message})
					if err != nil {
						logging.FromContext(ctx).Error().Err(err).Msg("error processing message")
						// dont ack
						continue
					}

					if _, err := b.redis.XAck(ctx, b.config.streamName, b.config.consumerGroup, xMessage.ID).Result(); err != nil {
						logging.FromContext(ctx).Error().Err(err).Msg("error ack messages")
					}

				default:
					logging.FromContext(ctx).Error().
						Err(fmt.Errorf("error invalid dataKey type messageID %s", xMessage.ID)).Msg("processing message")

					err := b.redis.SendRawMessage(ctx, xMessage.Values, b.deadLetterQueue)
					if err != nil {
						logging.FromContext(ctx).Error().Err(err).Msg("error decoding message")
					}
				}
			}
			timer.Reset(b.config.retryDelay)
		}
	}
}

// Option func to build batchConsumerConfig
type Option func(config *batchConsumerConfig) error

// NewBatchConsumerConfig return a new instance of batchConsumerConfig
func NewBatchConsumerConfig(options ...Option) (*batchConsumerConfig, error) {
	config := &batchConsumerConfig{
		start:       start,
		count:       count,
		block:       block,
		minIdleTime: minIdleTime,
		retryDelay:  retryDelay,
	}
	for _, option := range options {
		if err := option(config); err != nil {
			return nil, fmt.Errorf("error initializing batch consumer config %w", err)
		}
	}
	return config, nil
}

// WithStreamName sets the stream name
func WithStreamName(streamName string) Option {
	return func(b *batchConsumerConfig) error {
		b.streamName = streamName
		return nil
	}
}

// WithConsumerID sets the consumer id
func WithConsumerID(consumerID uuid.UUID) Option {
	return func(b *batchConsumerConfig) error {
		b.consumerID = consumerID
		return nil
	}
}

// WithConsumerGroup sets the consumer group
func WithConsumerGroup(consumerGroup string) Option {
	return func(b *batchConsumerConfig) error {
		b.consumerGroup = consumerGroup
		return nil
	}
}

// WithStart sets position to start processing messages. Default 0
func WithStart(start string) Option {
	return func(b *batchConsumerConfig) error {
		b.start = start
		return nil
	}
}

// WithCount sets the maximum number of messages to read from the stream. Default 10
func WithCount(count int64) Option {
	return func(b *batchConsumerConfig) error {
		b.count = count
		return nil
	}
}

// WithBlock sets the time.Duration a consumer should block when the stream is empty. Default 0
func WithBlock(block time.Duration) Option {
	return func(b *batchConsumerConfig) error {
		b.block = block
		return nil
	}
}

// WithMinIdleTime sets the minimum time to wait before a message a pending/failed message
// can be auto claimed for reprocess. Default 5 * time.Second
func WithMinIdleTime(minIdleTime time.Duration) Option {
	return func(b *batchConsumerConfig) error {
		b.minIdleTime = minIdleTime
		return nil
	}
}

// WithRetryDelay sets the minimum delay before polling the pending stream for non ack messages. Default 1 * time.Minute
func WithRetryDelay(retryDelay time.Duration) Option {
	return func(b *batchConsumerConfig) error {
		b.retryDelay = retryDelay
		return nil
	}
}

package event

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/rs/zerolog"

	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/go-redis/redis/v8"
	uuid "github.com/satori/go.uuid"
)

// busyGroup when a consumer group with the same name already exists redis returns a BUSYGROUP error.
const busyGroup = "BUSYGROUP"

// Default values used by BatchConsumerConfig.
const (
	// start is the id we begin processing at.
	start = "0"
	// count is the maximum number of messages to read from the stream.
	count = 10
	// block is the time.Duration a consumer should block when the stream is empty.
	block = 0
	//minIdleTime is the minimum time a message can should be idle before being claimed for reprocess.
	minIdleTime = 5 * time.Second
	// retryDelay is the minimum delay before polling the pending stream for non ack messages.
	retryDelay = 1 * time.Minute
	// maxRetry is the maximum number of times a pending message should be retried
	maxRetry = 5
)

type (
	// BatchConsumer defines the dependence's for a batch consumer.
	BatchConsumer struct {
		redis           *Client
		config          BatchConsumerConfig
		handler         Handler
		router          Router
		deadLetterQueue string
	}

	// BatchConsumerConfig defines the configuration to be used by the event.BatchConsumer.
	BatchConsumerConfig struct {
		streamName    string
		consumerID    uuid.UUID
		consumerGroup string
		start         string
		count         int64
		block         time.Duration
		minIdleTime   time.Duration
		retryDelay    time.Duration
		maxRetry      int64
	}

	// Handler defines a message handler.
	Handler interface {
		Handle(ctx context.Context, messages []Message) error
	}

	// Router defines a router function.
	Router func(message *Message) error
)

// NewBatchConsumer return a new instance batch consumer.
func NewBatchConsumer(redis *Client, config BatchConsumerConfig, handler Handler, router Router,
	deadLetterQueue string) (*BatchConsumer, error) {
	return &BatchConsumer{
		redis:           redis,
		config:          config,
		handler:         handler,
		router:          router,
		deadLetterQueue: deadLetterQueue,
	}, nil
}

// Consume connects or creates a new consumer group and starts processing the event.Message's.
func (b *BatchConsumer) Consume(ctx context.Context) error {
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
func (b *BatchConsumer) process(ctx context.Context) {

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
					Err(fmt.Errorf("error reading stream: %w", err)).
					Msg("batch consumer")
			}
			return
		default:

			streams, err := b.redis.XReadGroup(ctx, xReadGroupArgs).Result()
			if err != nil {
				logging.FromContext(ctx).Error().
					Err(fmt.Errorf("error reading messages: %w", err)).
					Msg("batch consumer")
			}

			// check we have xMessages to process
			if len(streams) < 1 {
				continue
			}

			var xMessageIDs []string
			var messages []Message

			for _, xMessage := range streams[0].Messages {

				logging.FromContext(ctx).Info().
					Str("x_message_id", xMessage.ID).
					Msg("batch consumer processing")

				d, ok := xMessage.Values[dataKey]
				if !ok {

					logging.FromContext(ctx).Error().
						Str("x_message_id", xMessage.ID).
						Err(fmt.Errorf("error retrieving message data: %w", err)).
						Msg("batch consumer processing")

					err := b.sendDeadLetter(ctx, xMessage)
					if err != nil {
						logging.FromContext(ctx).Error().
							Str("x_message_id", xMessage.ID).
							Err(errors.New("error sending message to dlq")).
							Msg("batch consumer processing")
					}
					continue
				}

				switch s := d.(type) {
				case string:
					xMessageIDs = append(xMessageIDs, xMessage.ID)

					message, err := NewMessageFromString(s)
					if err != nil || len(message.Body) == 0 {

						logging.FromContext(ctx).Error().
							Str("x_message_id", xMessage.ID).
							Err(fmt.Errorf("error creating new message: %w", err)).
							Msg("batch consumer processing")

						err := b.sendDeadLetter(ctx, xMessage)
						if err != nil {
							logging.FromContext(ctx).Error().
								Str("x_message_id", xMessage.ID).
								Err(errors.New("error sending message to dlq")).
								Msg("batch consumer processing")
						}
						continue
					}

					if _, ok := message.Headers[HeaderCorrelationID]; !ok {
						message.Headers[HeaderCorrelationID] = uuid.NewV4().String()
					}

					if b.router != nil {
						err := b.router(message)
						if err != nil {

							logging.FromContext(ctx).Error().
								Str("x_message_id", xMessage.ID).
								Str("message_id", message.ID.String()).
								Err(fmt.Errorf("error adding router: %w", err)).
								Msg("batch consumer processing")

							err := b.sendDeadLetter(ctx, xMessage)
							if err != nil {
								logging.FromContext(ctx).Error().
									Str("x_message_id", xMessage.ID).
									Err(errors.New("error sending message to dlq")).
									Msg("batch consumer processing")
							}
							continue
						}
					}

					messages = append(messages, *message)

				default:
					logging.FromContext(ctx).Error().
						Str("x_message_id", xMessage.ID).
						Err(fmt.Errorf("error invalid message data type %s", s)).
						Msg("batch consumer processing")

					err := b.sendDeadLetter(ctx, xMessage)
					if err != nil {
						logging.FromContext(ctx).Error().
							Str("x_message_id", xMessage.ID).
							Err(errors.New("error sending message to dlq")).
							Msg("batch consumer processing")
					}
				}
			}

			// check we have messages to process
			if len(messages) < 1 {
				continue
			}

			err = b.handler.Handle(ctx, messages)
			if err != nil {
				logging.FromContext(ctx).Error().
					Strs("message_ids", b.getIDs(messages)).
					Err(fmt.Errorf("error handling messages: %w", err)).
					Msg("batch consumer processing")
				// dont ack the messages so they can be retried
				continue
			}

			if _, err := b.redis.XAck(ctx, b.config.streamName, b.config.consumerGroup, xMessageIDs...).Result(); err != nil {
				// log the error and allow it to retry until we max out our retry attempts
				logging.FromContext(ctx).Error().
					Strs("message_ids", b.getIDs(messages)).
					Err(errors.New("failed to ack message")).
					Msg("batch consumer processing")
			}
		}
	}
}

// retry is responsible for consuming messaged that have failed to process first time
func (b *BatchConsumer) retry(ctx context.Context) {

	xPendingExtArgs := &redis.XPendingExtArgs{
		Stream: b.config.streamName,
		Group:  b.config.consumerGroup,
		Idle:   b.config.minIdleTime,
		Start:  "-",
		End:    "+",
		Count:  b.config.count,
	}

	timer := time.NewTimer(b.config.retryDelay)

	for {

		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				logging.FromContext(ctx).Error().
					Err(fmt.Errorf("error reading stream: %w", err)).
					Msg("batch consumer retry")
			}
			return
		default:

			xPending, err := b.redis.XPendingExt(ctx, xPendingExtArgs).Result()
			if err != nil {
				logging.FromContext(ctx).Error().
					Err(fmt.Errorf("error getting pending messages: %w", err)).
					Msg("batch consumer retry")
				continue
			}

			// nothing pending
			numPending := len(xPending)
			if numPending == 0 {
				continue
			}

			xPendingID := make([]string, numPending)
			xPendingRetryCount := make(map[string]int64, numPending)

			for i := 0; i < numPending; i++ {
				xPendingID[i] = xPending[i].ID
				xPendingRetryCount[xPending[i].ID] = xPending[i].RetryCount
			}

			xMessages, err := b.redis.XClaim(ctx, &redis.XClaimArgs{
				Stream:   b.config.streamName,
				Group:    b.config.consumerGroup,
				Consumer: b.config.consumerID.String(),
				MinIdle:  b.config.minIdleTime,
				Messages: xPendingID,
			}).Result()

			if err != nil {
				logging.FromContext(ctx).Error().
					Strs("x_pending_ids", xPendingID).
					Err(fmt.Errorf("error claiming pending messages: %w", err)).
					Msg("batch consumer retry")
			}

			for _, xMessage := range xMessages {
				// a message may have been claimed by another consumer after calling xPending
				// so check if we claimed it successfully before further processing if not skip it
				retryCount, ok := xPendingRetryCount[xMessage.ID]
				if !ok {
					continue
				}

				// if we have reached max retries for this message then send to dlq
				if retryCount > b.config.maxRetry {
					logging.FromContext(ctx).Error().
						Str("x_message_id", xMessage.ID).
						Err(errors.New("max retries reached")).
						Msg("batch consumer retry")

					err := b.sendDeadLetter(ctx, xMessage)
					if err != nil {
						logging.FromContext(ctx).Error().
							Str("x_message_id", xMessage.ID).
							Err(errors.New("error sending message to dlq")).
							Msg("batch consumer retry")
					}
					continue
				}

				d, ok := xMessage.Values[dataKey]
				if !ok {
					logging.FromContext(ctx).Error().
						Str("x_message_id", xMessage.ID).
						Err(fmt.Errorf("error retrieving message data: %w", err)).
						Msg("batch consumer retry")

					err := b.sendDeadLetter(ctx, xMessage)
					if err != nil {
						logging.FromContext(ctx).Error().
							Str("x_message_id", xMessage.ID).
							Err(errors.New("error sending message to dlq")).
							Msg("batch consumer retry")
					}
					continue
				}

				switch s := d.(type) {
				case string:

					message, err := NewMessageFromString(s)
					if err != nil {
						logging.FromContext(ctx).Error().
							Str("x_message_id", xMessage.ID).
							Err(fmt.Errorf("error creating new message: %w", err)).
							Msg("batch consumer retry")

						err := b.sendDeadLetter(ctx, xMessage)
						if err != nil {
							logging.FromContext(ctx).Error().
								Str("x_message_id", xMessage.ID).
								Err(errors.New("error sending message to dlq")).
								Msg("batch consumer retry")
						}
						continue
					}

					if _, ok := message.Headers[HeaderCorrelationID]; !ok {
						message.Headers[HeaderCorrelationID] = uuid.NewV4().String()
					}

					if b.router != nil {
						err := b.router(message)
						if err != nil {

							logging.FromContext(ctx).Error().
								Str("x_message_id", xMessage.ID).
								Str("message_id", message.ID.String()).
								Err(fmt.Errorf("error adding router: %w", err)).
								Msg("batch consumer retry")

							err := b.sendDeadLetter(ctx, xMessage)
							if err != nil {
								logging.FromContext(ctx).Error().
									Str("x_message_id", xMessage.ID).
									Err(errors.New("error sending message to dlq")).
									Msg("batch consumer retry")
							}
							continue
						}
					}

					err = b.handler.Handle(ctx, []Message{*message})
					if err != nil {
						logging.FromContext(ctx).Error().
							Str("x_message_id", xMessage.ID).
							Str("message_id", message.ID.String()).
							Err(fmt.Errorf("error handling message: %w", err)).
							Msg("batch consumer retry")
						// dont ack the message and keep retrying until we max out our retry attempts
						continue
					}

					if _, err := b.redis.XAck(ctx, b.config.streamName, b.config.consumerGroup, xMessage.ID).Result(); err != nil {
						// log the error and allow it to retry until we max out our retry attempts
						logging.FromContext(ctx).Error().
							Str("x_message_id", xMessage.ID).
							Str("message_id", message.ID.String()).
							Err(errors.New("failed to ack message")).
							Msg("batch consumer retry")
					}

				default:
					logging.FromContext(ctx).Error().
						Str("x_message_id", xMessage.ID).
						Err(fmt.Errorf("error invalid message data type %s", s)).
						Msg("batch consumer retry")

					err := b.sendDeadLetter(ctx, xMessage)
					if err != nil {
						logging.FromContext(ctx).Error().
							Str("x_message_id", xMessage.ID).
							Err(errors.New("error sending message to dlq")).
							Msg("batch consumer retry")
					}
				}
			}
			timer.Reset(b.config.retryDelay)
		}
	}
}

// sendDeadLetter creates a new deadletter Message with the given xMessage then send and ack the message.
// Informs sentry a message has failed to process
func (b *BatchConsumer) sendDeadLetter(ctx context.Context, xMessage redis.XMessage) error {

	deadletter, err := NewMessage(Deadletter, xMessage.Values)
	if err != nil {
		return fmt.Errorf("error creating new message: %w", err)
	}

	// add failed on headers to help debugging
	deadletter.Headers["X-Failed-On-Consumer-Group"] = b.config.consumerGroup
	deadletter.Headers["X-Failed-On-Stream-Name"] = b.config.streamName

	err = b.redis.Send(ctx, *deadletter, b.deadLetterQueue)
	if err != nil {
		return fmt.Errorf("error sending message: %w", err)
	}

	_, err = b.redis.XAck(ctx, b.config.streamName, b.config.consumerGroup, xMessage.ID).Result()
	if err != nil {
		return fmt.Errorf("error acknowledging message: %w", err)
	}

	defer func() {
		var exception = fmt.Errorf("error sending %s message to dql: %w", xMessage.ID, err)
		// only log successful dlq attempt
		if err == nil {
			exception = fmt.Errorf("message %s sent to dql", xMessage.ID)
			logging.FromContext(ctx).Error().
				Str("x_message_id", xMessage.ID).
				Err(exception).
				Msg("batch consumer retry")
		}
		sentry.CaptureException(exception)
	}()

	return nil
}

func (b *BatchConsumer) getIDs(messages []Message) []string {
	ids := make([]string, len(messages))
	for i := 0; i < len(messages); i++ {
		ids[i] = messages[i].ID.String()
	}
	return ids
}

// Option func to build BatchConsumerConfig.
type Option func(config *BatchConsumerConfig) error

// NewBatchConsumerConfig return a new instance of BatchConsumerConfig.
func NewBatchConsumerConfig(options ...Option) (*BatchConsumerConfig, error) {
	config := &BatchConsumerConfig{
		start:       start,
		count:       count,
		block:       block,
		minIdleTime: minIdleTime,
		retryDelay:  retryDelay,
		maxRetry:    maxRetry,
	}
	for _, option := range options {
		if err := option(config); err != nil {
			return nil, fmt.Errorf("error initializing batch consumer config %w", err)
		}
	}
	return config, nil
}

// WithStreamName sets the stream name.
func WithStreamName(streamName string) Option {
	return func(b *BatchConsumerConfig) error {
		b.streamName = streamName
		return nil
	}
}

// WithConsumerID sets the consumer id.
func WithConsumerID(consumerID uuid.UUID) Option {
	return func(b *BatchConsumerConfig) error {
		b.consumerID = consumerID
		return nil
	}
}

// WithConsumerGroup sets the consumer group.
func WithConsumerGroup(consumerGroup string) Option {
	return func(b *BatchConsumerConfig) error {
		b.consumerGroup = consumerGroup
		return nil
	}
}

// WithStart sets position to start processing messages. Default 0.
func WithStart(start string) Option {
	return func(b *BatchConsumerConfig) error {
		b.start = start
		return nil
	}
}

// WithCount sets the maximum number of messages to read from the stream. Default 10.
func WithCount(count int64) Option {
	return func(b *BatchConsumerConfig) error {
		b.count = count
		return nil
	}
}

// WithBlock sets the time.Duration a consumer should block when the stream is empty. Default 0.
func WithBlock(block time.Duration) Option {
	return func(b *BatchConsumerConfig) error {
		b.block = block
		return nil
	}
}

// WithMinIdleTime sets the minimum time to wait before a message a pending/failed message
// can be auto claimed for reprocess. Default 5 * time.Second.
func WithMinIdleTime(minIdleTime time.Duration) Option {
	return func(b *BatchConsumerConfig) error {
		b.minIdleTime = minIdleTime
		return nil
	}
}

// WithRetryDelay sets the minimum delay before polling the pending stream for non ack messages. Default 1 * time.Minute.
func WithRetryDelay(retryDelay time.Duration) Option {
	return func(b *BatchConsumerConfig) error {
		b.retryDelay = retryDelay
		return nil
	}
}

// WithMaxRetry sets the maximum number of times a pending message should be retried. Default 5.
func WithMaxRetry(maxRetry int64) Option {
	return func(b *BatchConsumerConfig) error {
		b.maxRetry = maxRetry
		return nil
	}
}

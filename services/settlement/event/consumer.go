package event

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/logging"

	"github.com/getsentry/sentry-go"
	"github.com/go-redis/redis/v8"
)

// busyGroup when a consumer group with the same name already exists redis returns a BUSYGROUP error.
const busyGroup = "BUSYGROUP"

// maxRetryDisabled this flag disables the maximum retry for pending messages.
const maxRetryDisabled = -1

// Default values used by BatchConsumerConfig.
const (
	// start is the id we begin processing at.
	start = "0"
	// count is the maximum number of messages to read from the stream.
	count = 20
	// block is the time.Duration a consumer should block when the stream is empty.
	block = 0
	//minIdleTime is the minimum time a message can be idle before being claimed for reprocess.
	minIdleTime = 5 * time.Second
	// maxRetry is the maximum number of times a pending message should be retried.
	// maxRetry is disabled by default.
	maxRetry = maxRetryDisabled
	// cacheLimit is the number of messages that should be acked at a time.
	cacheLimit = 10
	// cacheTimeout is the time to wait before calling ack message.
	cacheTimeout = 1 * time.Second
	// statusTimout is the time to wait before checking if all messages have been processed for a given stream.
	statusTimout = 10 * time.Second
	// retryAfterPrefix prefix used when setting retry after key-value.
	retryAfterPrefix = "retry-after"
)

type (
	// Consumer defines the method signatures for a consumer.
	Consumer interface {
		Start(ctx context.Context, resultC chan<- error) error
		Del(ctx context.Context) (int64, error)
	}

	// BatchConsumer defines the dependence's for a BatchConsumer.
	BatchConsumer struct {
		redis        *RedisClient
		config       BatchConsumerConfig
		handler      Handler
		errorHandler ErrorHandler
	}

	// BatchConsumerConfig defines the configuration to be used by the BatchConsumer.
	BatchConsumerConfig struct {
		streamName    string
		consumerID    string
		consumerGroup string
		start         string
		count         int64
		block         time.Duration
		minIdleTime   time.Duration
		maxRetry      int64
		cacheLimit    int
		cacheTimeout  time.Duration
		statusTimeout time.Duration
	}

	// Handler defines the method signatures for a message Handler.
	Handler interface {
		Handle(ctx context.Context, message Message) error
	}

	// ErrorHandler defines the method signatures for an ErrorHandler.
	ErrorHandler interface {
		Handle(ctx context.Context, xMessage redis.XMessage, processingError error) error
	}
)

var (
	errDataKeyNotFound            = errors.New("data key not found")
	errMaxRetriesReached          = errors.New("max retries reached")
	ErrConsumerHasPendingMessages = errors.New("consumer has pending messages")
)

// RetryError defines the fields for a retry error. This error can be returned by handlers
// and the message will not be retried until the RetryAfter time has expired.
type RetryError struct {
	RetryAfter time.Duration
}

// Error returns the RetryError error message.
func (r RetryError) Error() string {
	return "retry after " + r.RetryAfter.String()
}

// NewBatchConsumer return a new instance of BatchConsumer.
func NewBatchConsumer(redis *RedisClient, config BatchConsumerConfig, handler Handler, errorHandler ErrorHandler) Consumer {
	return &BatchConsumer{
		redis:        redis,
		config:       config,
		handler:      handler,
		errorHandler: errorHandler,
	}
}

// Start creates or connects to an existing consumer group and starts processing event.Message's.
func (b *BatchConsumer) Start(ctx context.Context, resultC chan<- error) error {
	// Create or connect to an existing consumer group.
	_, err := b.redis.XGroupCreateMkStream(ctx, b.config.streamName, b.config.consumerGroup, b.config.start).Result()
	if err != nil && !strings.Contains(err.Error(), busyGroup) {
		return fmt.Errorf("error creating consumer group %w", err)
	}

	// Start the message processing routines.
	processC := make(chan string)
	retryC := make(chan string)
	b.processAsync(ctx, processC)
	b.retryAsync(ctx, retryC)

	ackC := NewFanIn[string]()(ctx, processC, retryC)
	b.ackAsync(ctx, ackC)

	b.statusAsync(ctx, resultC)

	return nil
}

// Del removes the consumer from the group. A consumer can only be deleted if it does not have pending messages.
func (b *BatchConsumer) Del(ctx context.Context) (int64, error) {
	//TODO needs investigation on how to achieve atomic operation.
	return 0, ErrConsumerHasPendingMessages
}

func (b *BatchConsumer) processAsync(ctx context.Context, ackC chan<- string) {
	go func() {
		defer close(ackC)

		logger := logging.Logger(ctx, "BatchConsumer.processAsync "+b.config.String())

		xReadGroupArgs := &redis.XReadGroupArgs{
			Streams:  []string{b.config.streamName, ">"},
			Group:    b.config.consumerGroup,
			Consumer: b.config.consumerID,
			Count:    b.config.count,
			Block:    b.config.block,
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
				streams, err := b.redis.XReadGroup(ctx, xReadGroupArgs).Result()
				if err != nil {
					logger.Error().Err(err).Msg("error xReadGroup")
				}

				// Check we have messages to process.
				if len(streams) <= 0 {
					continue
				}

				for _, xMessage := range streams[0].Messages {
					logger.Info().
						Str("x_message_id", xMessage.ID).
						Msg("")

					data, ok := xMessage.Values[dataKey]
					if !ok {
						err := b.errorHandler.Handle(ctx, xMessage, errDataKeyNotFound)
						if err != nil {
							logger.Error().Err(err).
								Str("x_message_id", xMessage.ID).
								Msg("error data handler")
							sentry.CaptureException(err)
							continue
						}
						ackC <- xMessage.ID
						continue
					}

					switch s := data.(type) {
					case string:
						message, err := NewMessageFromString(s)
						if err != nil || len(message.Body) == 0 {
							err := b.errorHandler.Handle(ctx, xMessage, err)
							if err != nil {
								logger.Error().Err(err).
									Str("x_message_id", xMessage.ID).
									Msg("error new message")
								sentry.CaptureException(err)
								continue
							}
							ackC <- xMessage.ID
							continue
						}

						switch err := b.handler.Handle(ctx, *message).(type) {
						case nil:
							ackC <- xMessage.ID
						case RetryError:
							if _, err := b.redis.Set(ctx, retryAfterPrefix+message.ID.String(), "",
								err.RetryAfter).Result(); err != nil {
								logger.Error().Err(err).Msg("error setting retry after")
							}
						default:
							err = b.errorHandler.Handle(ctx, xMessage, err)
							if err != nil {
								logger.Error().Err(err).
									Str("x_message_id", xMessage.ID).
									Msg("error handling")
								sentry.CaptureException(err)
								continue
							}
							ackC <- xMessage.ID
						}

					default:
						err := b.errorHandler.Handle(ctx, xMessage, fmt.Errorf("unknow data type: %+v", s))
						if err != nil {
							logger.Error().Err(err).
								Str("x_message_id", xMessage.ID).
								Msg("error handling")
							sentry.CaptureException(err)
							continue
						}
						ackC <- xMessage.ID
					}
				}
			}
		}
	}()
}

//func (b *BatchConsumer) processAsync(ctx context.Context) <-chan string {
//	ackC := make(chan string)
//	go func() {
//		defer close(ackC)
//
//		logger := logging.Logger(ctx, "BatchConsumer.processAsync "+b.config.String())
//
//		xReadGroupArgs := &redis.XReadGroupArgs{
//			Streams:  []string{b.config.streamName, ">"},
//			Group:    b.config.consumerGroup,
//			Consumer: b.config.consumerID,
//			Count:    b.config.count,
//			Block:    b.config.block,
//		}
//
//		for {
//			select {
//			case <-ctx.Done():
//				return
//			default:
//				streams, err := b.redis.XReadGroup(ctx, xReadGroupArgs).Result()
//				if err != nil {
//					logger.Error().Err(err).Msg("error xReadGroup")
//				}
//
//				// Check we have messages to process.
//				if len(streams) <= 0 {
//					continue
//				}
//
//				for _, xMessage := range streams[0].Messages {
//					logger.Info().
//						Str("x_message_id", xMessage.ID).
//						Msg("")
//
//					data, ok := xMessage.Values[dataKey]
//					if !ok {
//						err := b.errorHandler.Handle(ctx, xMessage, errDataKeyNotFound)
//						if err != nil {
//							logger.Error().Err(err).
//								Str("x_message_id", xMessage.ID).
//								Msg("error data handler")
//							sentry.CaptureException(err)
//							continue
//						}
//						ackC <- xMessage.ID
//						continue
//					}
//
//					switch s := data.(type) {
//					case string:
//						message, err := NewMessageFromString(s)
//						if err != nil || len(message.Body) == 0 {
//							err := b.errorHandler.Handle(ctx, xMessage, err)
//							if err != nil {
//								logger.Error().Err(err).
//									Str("x_message_id", xMessage.ID).
//									Msg("error new message")
//								sentry.CaptureException(err)
//								continue
//							}
//							ackC <- xMessage.ID
//							continue
//						}
//
//						switch err := b.handler.Handle(ctx, *message).(type) {
//						case nil:
//							ackC <- xMessage.ID
//						case RetryError:
//							if _, err := b.redis.Set(ctx, retryAfterPrefix+message.ID.String(), "",
//								err.RetryAfter).Result(); err != nil {
//								logger.Error().Err(err).Msg("error setting retry after")
//							}
//						default:
//							err = b.errorHandler.Handle(ctx, xMessage, err)
//							if err != nil {
//								logger.Error().Err(err).
//									Str("x_message_id", xMessage.ID).
//									Msg("error handling")
//								sentry.CaptureException(err)
//								continue
//							}
//							ackC <- xMessage.ID
//						}
//
//					default:
//						err := b.errorHandler.Handle(ctx, xMessage, fmt.Errorf("unknow data type: %+v", s))
//						if err != nil {
//							logger.Error().Err(err).
//								Str("x_message_id", xMessage.ID).
//								Msg("error handling")
//							sentry.CaptureException(err)
//							continue
//						}
//						ackC <- xMessage.ID
//					}
//				}
//			}
//		}
//	}()
//	return ackC
//}

func (b *BatchConsumer) retryAsync(ctx context.Context, ackC chan<- string) {
	go func() {
		defer close(ackC)

		logger := logging.Logger(ctx, "BatchConsumer.retryAsync "+b.config.String())

		xPendingExtArgs := &redis.XPendingExtArgs{
			Stream: b.config.streamName,
			Group:  b.config.consumerGroup,
			Idle:   b.config.minIdleTime,
			Start:  "-",
			End:    "+",
			Count:  b.config.count,
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
				xPending, err := b.redis.XPendingExt(ctx, xPendingExtArgs).Result()
				if err != nil {
					logger.Error().Err(err).
						Msg("error getting pending messages")
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
					Consumer: b.config.consumerID,
					MinIdle:  b.config.minIdleTime,
					Messages: xPendingID,
				}).Result()
				if err != nil {
					logger.Error().Err(err).
						Strs("x_pending_ids", xPendingID).
						Msg("error claiming pending messages")
				}

				for _, xMessage := range xMessages {
					// a message may have been claimed by another consumer after calling xPending
					// so check if we claimed it successfully before further processing if not skip it.
					retryCount, ok := xPendingRetryCount[xMessage.ID]
					if !ok {
						continue
					}

					// if max retry is enabled, and we have reached max retries for this message then send to dlq.
					if b.config.maxRetry != maxRetryDisabled && retryCount > b.config.maxRetry {
						err := b.errorHandler.Handle(ctx, xMessage, errMaxRetriesReached)
						if err != nil {
							logger.Error().Err(err).
								Str("x_message_id", xMessage.ID).
								Msg("error handling max retries")
							sentry.CaptureException(err)
							continue
						}
						ackC <- xMessage.ID
						continue
					}

					d, ok := xMessage.Values[dataKey]
					if !ok {
						err := b.errorHandler.Handle(ctx, xMessage, errDataKeyNotFound)
						if err != nil {
							logger.Error().Err(err).
								Str("x_message_id", xMessage.ID).
								Msg("error data handler")
							sentry.CaptureException(err)
							continue
						}
						ackC <- xMessage.ID
						continue
					}

					switch s := d.(type) {
					case string:

						message, err := NewMessageFromString(s)
						if err != nil || len(message.Body) == 0 {
							err := b.errorHandler.Handle(ctx, xMessage, err)
							if err != nil {
								logger.Error().Err(err).
									Str("x_message_id", xMessage.ID).
									Msg("error new message")
								sentry.CaptureException(err)
								continue
							}
							ackC <- xMessage.ID
							continue
						}

						// Check to see if there is a retry after value, if no key exists then we can process the message again.
						_, err = b.redis.Get(ctx, retryAfterPrefix+message.ID.String()).Result()
						if err != nil {
							if !errors.Is(err, redis.Nil) {
								logger.Error().Err(err).
									Msg("error getting retry after value")
								continue
							}
						}

						switch err := b.handler.Handle(ctx, *message).(type) {
						case nil:
							ackC <- xMessage.ID
						case RetryError:
							if _, err := b.redis.Set(ctx, retryAfterPrefix+message.ID.String(), "",
								err.RetryAfter).Result(); err != nil {
								logger.Error().Err(err).Msg("error setting retry after")
							}
						default:
							err = b.errorHandler.Handle(ctx, xMessage, err)
							if err != nil {
								logger.Error().Err(err).
									Str("x_message_id", xMessage.ID).
									Msg("error handling")
								sentry.CaptureException(err)
								continue
							}
							ackC <- xMessage.ID
						}

					default:
						err := b.errorHandler.Handle(ctx, xMessage, fmt.Errorf("unknow data type: %+v", s))
						if err != nil {
							logger.Error().Err(err).
								Str("x_message_id", xMessage.ID).
								Msg("error handling")
							sentry.CaptureException(err)
							continue
						}
						ackC <- xMessage.ID
					}
				}
			}
		}
	}()
}

// ackAsync periodically ack the messages for the consumer.
func (b *BatchConsumer) ackAsync(ctx context.Context, ackC <-chan string) {
	go func() {
		logger := logging.Logger(ctx, "BatchConsumer.ackAsync")

		cache := make([]string, 0, b.config.cacheLimit)
		ticker := time.NewTicker(b.config.cacheTimeout)

		ackFn := func(cache []string) error {
			if _, err := b.redis.XAck(ctx, b.config.streamName, b.config.consumerGroup, cache...).Result(); err != nil {
				return err
			}
			cache = cache[:0]
			return nil
		}

		for {
			select {
			case <-ctx.Done():
				return
			case xMessageID := <-ackC:
				cache = append(cache, xMessageID)
				if len(cache) < b.config.cacheLimit {
					break
				}

				ticker.Stop()
				err := ackFn(cache)
				if err != nil {
					// log the error and allow it to retry until we max out our retry attempts
					logger.Error().Err(err).
						Strs("message_ids", cache).
						Msg("batch consumer processing")
					sentry.CaptureException(err)
				}

				ticker = time.NewTicker(b.config.cacheTimeout)

			case <-ticker.C:
				if len(cache) < 1 {
					break
				}

				err := ackFn(cache)
				if err != nil {
					// log the error and allow it to retry until we max out our retry attempts
					logger.Error().Err(err).
						Strs("message_ids", cache).
						Msg("batch consumer processing")
					sentry.CaptureException(err)
				}
			}
		}
	}()
}

// statusAsync periodically check if all messages have been processed.
// Once all messages have been processed it sends a nil error on the resultC channel.
// Where both entries-read is equal to the count or len stream and pending is 0 then
// entries-read: the logical "read counter" of the last entry delivered to group's consumers
// pending: the length of the group's pending entries list (PEL), which are messages that were delivered but are yet
// to be acknowledged.
func (b *BatchConsumer) statusAsync(ctx context.Context, resultC chan<- error) {
	go func() {
		defer close(resultC)

		logger := logging.Logger(ctx, "BatchConsumer.statusAsync")

		ticker := time.NewTicker(b.config.statusTimeout)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				xInfoGroups, err := b.redis.XInfoGroups(ctx, b.config.streamName).Result()
				if err != nil {
					fmt.Println("call ", err)
				}

				// This is a workaround for not being able to check the cg lag.
				lastMessage, err := b.redis.XRevRangeN(ctx, b.config.streamName, "+", "-", 1).Result()
				if err != nil {
					logger.Error().Err(err).Msg("error calling xrevrange")
				}

				if len(xInfoGroups) == 0 || len(lastMessage) == 0 {
					continue
				}

				// Currently we cannot get the lag, so we need to check the ID of the last entry delivered to the consumer group,
				// if this matches the last xMessageID in the stream we can assume all messages have been delivered.
				if xInfoGroups[0].Pending == 0 && xInfoGroups[0].LastDeliveredID == lastMessage[0].ID {
					resultC <- nil
				}
			}
		}
	}()
}

type DLQErrorHandler struct {
	redis  *RedisClient
	config BatchConsumerConfig
	dlq    string
}

func NewDLQErrorHandler(client *RedisClient, config BatchConsumerConfig, dlq string) ErrorHandler {
	return &DLQErrorHandler{
		redis:  client,
		config: config,
		dlq:    dlq,
	}
}

// Handle implements a DQL handler. ExceptionHandler creates a new deadletter Message with the given
// xMessage then send and ack the message. Informs sentry a message has failed to process.
func (d *DLQErrorHandler) Handle(ctx context.Context, xMessage redis.XMessage, processingError error) (err error) {
	logger := logging.Logger(ctx, "DLQErrorHandler.Handle")

	m, err := NewMessage(xMessage.Values)
	if err != nil {
		return fmt.Errorf("error creating new dlq message: %w", err)
	}

	m.Headers["X-Error-On-Consumer-Group"] = d.config.consumerGroup
	m.Headers["X-Error-On-Stream-Name"] = d.config.streamName
	m.Headers["X-Error-Message"] = processingError.Error()

	err = d.redis.Send(ctx, d.dlq, m)
	if err != nil {
		return fmt.Errorf("error sending dlq message: %w", err)
	}

	logger.Error().Err(processingError).
		Str("x_message_id", xMessage.ID).
		Msg("message sent dlq")
	sentry.CaptureException(err)

	return nil
}

// Option func to build BatchConsumerConfig.
type Option func(config *BatchConsumerConfig) error

// NewBatchConsumerConfig return a new instance of BatchConsumerConfig.
func NewBatchConsumerConfig(options ...Option) (*BatchConsumerConfig, error) {
	config := &BatchConsumerConfig{
		start:         start,
		count:         count,
		block:         block,
		minIdleTime:   minIdleTime,
		maxRetry:      maxRetry,
		cacheLimit:    cacheLimit,
		cacheTimeout:  cacheTimeout,
		statusTimeout: statusTimout,
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
func WithConsumerID(consumerID string) Option {
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

// WithMaxRetry sets the maximum number of times a pending message should be retried. Default 5.
// MaxRetryEnabled must be set to true for this to take effect.
func WithMaxRetry(maxRetry int64) Option {
	return func(b *BatchConsumerConfig) error {
		b.maxRetry = maxRetry
		return nil
	}
}

// WithCacheLimit sets the maximum number of message to batch before calling ack. Default 10.
func WithCacheLimit(cacheLimit int) Option {
	return func(b *BatchConsumerConfig) error {
		b.cacheLimit = cacheLimit
		return nil
	}
}

// WithCacheTimeout sets the amount of time to wait before calling ack. Default 1 * time.Second.
func WithCacheTimeout(cacheTimeout time.Duration) Option {
	return func(b *BatchConsumerConfig) error {
		b.cacheTimeout = cacheTimeout
		return nil
	}
}

// WithStatusTimeout sets the amount of time to wait before checking if all messages have been processed for a
// given stream. Default 10 * time.Second.
func WithStatusTimeout(statusTimeout time.Duration) Option {
	return func(b *BatchConsumerConfig) error {
		b.statusTimeout = statusTimeout
		return nil
	}
}

func (b *BatchConsumerConfig) String() string {
	return fmt.Sprintf("id=%s cg=%s stream=%s", b.consumerID, b.consumerGroup, b.streamName)
}

package consumer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer/concurrent"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer/redis"
	"github.com/getsentry/sentry-go"
)

// maxRetryDisabled this flag disables the maximum retry for pending messages.
const maxRetryDisabled = -1

// Default values used by StreamConsumerConfig.
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
	// dataKey is the key used to retrieve the stream.Message value from the redis stream message.
	dataKey = "data"
)

type (
	// Consumer defines the methods signatures for a consumer.
	Consumer interface {
		Consume(ctx context.Context) error
	}

	// Handler defines the method signatures for a message Handler.
	Handler interface {
		Handle(ctx context.Context, message Message) error
	}

	// ErrorHandler defines the method signatures for an ErrorHandler.
	ErrorHandler interface {
		Handle(ctx context.Context, rawMessage redis.XMessage, processingError error) error
	}

	// RedisClient defines the method signatures used to interact with Redis.
	RedisClient interface {
		XGroupCreateMKStream(ctx context.Context, stream, group, start string) error
		XReadGroup(ctx context.Context, args *redis.XReadGroupArgs) ([]redis.XMessage, error)
		Set(ctx context.Context, args redis.SetArgs) (string, error)
		Get(ctx context.Context, key string) (string, error)
		XPending(ctx context.Context, args *redis.XPendingArgs) ([]redis.XPendingEntry, error)
		XClaim(ctx context.Context, args redis.XClaimArgs) ([]redis.XMessage, error)
		XAck(ctx context.Context, stream string, group string, ids ...string) error
		XInfoGroup(ctx context.Context, stream, group string) (redis.XInfoGroup, error)
		GetLastMessage(ctx context.Context, stream string) (redis.XMessage, error)
	}

	// consumer defines the dependence's for a StreamConsumer.
	consumer struct {
		redis   RedisClient
		conf    *Config
		handler Handler
		error   ErrorHandler
	}
)

var (
	errDataKeyNotFound   = errors.New("consumer: data key not found")
	errMaxRetriesReached = errors.New("consumer: max retries reached")
	errNoMsgBody         = errors.New("consumer: no message body")
)

// New returns a new Consumer.
func New(redis RedisClient, conf *Config, handler Handler, errorHandler ErrorHandler) Consumer {
	return &consumer{
		redis:   redis,
		conf:    conf,
		handler: handler,
		error:   errorHandler,
	}
}

// Consume creates or joins an existing consumer group and starts consuming Message's.
func (c *consumer) Consume(ctx context.Context) error {
	err := c.redis.XGroupCreateMKStream(ctx, c.conf.streamName, c.conf.consumerGroup, c.conf.start)
	if err != nil {
		return fmt.Errorf("error creating consumer group %w", err)
	}

	processC := c.processAsync(ctx)
	retryC := c.retryAsync(ctx)
	ackC := concurrent.NewFanIn[string]()(ctx, processC, retryC)
	c.ackAsync(ctx, ackC)

	resultC := c.statusAsync(ctx)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err = <-resultC:
		return err
	}
}

func (c *consumer) processAsync(ctx context.Context) <-chan string {
	ackC := make(chan string)
	go func() {
		defer close(ackC)

		l := logging.Logger(ctx, "Consumer.processAsync "+c.conf.String())

		readGroupArgs := &redis.XReadGroupArgs{
			Stream:   c.conf.streamName,
			Group:    c.conf.consumerGroup,
			Consumer: c.conf.consumerID,
			Count:    c.conf.count,
			Block:    c.conf.block,
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
				xMsgs, err := c.redis.XReadGroup(ctx, readGroupArgs)
				if err != nil {
					l.Error().Err(err).Msg("error xReadGroup")
					continue
				}

				// check we have msgs to process.
				if len(xMsgs) <= 0 {
					continue
				}

				for i := range xMsgs {
					l.Info().Str("x_message_id", xMsgs[i].ID).Msg("")

					message, err := newMessage(xMsgs[i])
					if err != nil {
						if err := c.error.Handle(ctx, xMsgs[i], err); err != nil {
							l.Error().Err(err).Str("x_message_id", xMsgs[i].ID).Msg("error data handler")
							sentry.CaptureException(err)
							continue
						}
						ackC <- xMsgs[i].ID
						continue
					}

					if err := c.handler.Handle(ctx, message); err != nil {
						l.Error().Err(err).Msg("handler error")
						var reErr *RetryError
						switch {
						case errors.As(err, &reErr):
							if err := setRetryAfter(ctx, c.redis, message.ID.String(), reErr.RetryDelay()); err != nil {
								l.Error().Err(err).Msg("error setting retry after")
							}
							continue
						default:
							if err := c.error.Handle(ctx, xMsgs[i], err); err != nil {
								l.Error().Err(err).Str("x_message_id", xMsgs[i].ID).Msg("error handling error")
								sentry.CaptureException(err)
								continue
							}
						}
					}
					ackC <- xMsgs[i].ID
				}
			}
		}
	}()
	return ackC
}

func (c *consumer) retryAsync(ctx context.Context) <-chan string {
	ackC := make(chan string)
	go func() {
		defer close(ackC)

		l := logging.Logger(ctx, "Consumer.retryAsync "+c.conf.String())

		pendingArgs := &redis.XPendingArgs{
			Stream: c.conf.streamName,
			Group:  c.conf.consumerGroup,
			Idle:   c.conf.minIdleTime,
			Count:  c.conf.count,
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
				pendingEntries, err := c.redis.XPending(ctx, pendingArgs)
				if err != nil {
					l.Error().Err(err).Msg("error getting pending entries")
					continue
				}

				countPending := len(pendingEntries)
				if countPending == 0 {
					continue
				}

				// TODO(clD11) move this into function

				// flatten the pending entry ids for use in the claim command
				pendingEntryIDs := make([]string, countPending)
				// keep a map of pending entry to retry count, so we can check it against the claimed message
				pendingEntryRetryCount := make(map[string]int64, countPending)

				for i := 0; i < countPending; i++ {
					pendingEntryIDs[i] = pendingEntries[i].ID
					pendingEntryRetryCount[pendingEntries[i].ID] = pendingEntries[i].RetryCount
				}

				xMsgs, err := c.redis.XClaim(ctx, redis.XClaimArgs{
					Stream:   c.conf.streamName,
					Group:    c.conf.consumerGroup,
					Consumer: c.conf.consumerID,
					MinIdle:  c.conf.minIdleTime,
					Messages: pendingEntryIDs,
				})
				if err != nil {
					l.Error().Err(err).Strs("x_pending_ids", pendingEntryIDs).Msg("error claiming pendingEntries messages")
					continue
				}

				for i := range xMsgs {
					// A message may have been claimed by another consumer after calling XPending
					// so check if we claimed it successfully before further processing if not skip it.
					// TODO(clD11) check if we can remove this check as pending will always contain the xMsg
					retryCount, ok := pendingEntryRetryCount[xMsgs[i].ID]
					if !ok {
						continue
					}

					// If max retry is enabled, and we have reached max retries for this message then send to dlq.
					if c.conf.maxRetry != maxRetryDisabled && retryCount > c.conf.maxRetry {
						if err := c.error.Handle(ctx, xMsgs[i], errMaxRetriesReached); err != nil {
							l.Error().Err(err).Str("x_message_id", xMsgs[i].ID).Msg("error handling max retries")
							sentry.CaptureException(err)
							continue
						}
						ackC <- xMsgs[i].ID
						continue
					}

					message, err := newMessage(xMsgs[i])
					if err != nil {
						if err := c.error.Handle(ctx, xMsgs[i], err); err != nil {
							l.Error().Err(err).Str("x_message_id", xMsgs[i].ID).Msg("error data handler")
							sentry.CaptureException(err)
							continue
						}
						ackC <- xMsgs[i].ID
						continue
					}

					retry, err := fetchRetryAfter(ctx, c.redis, message.ID.String())
					if err != nil {
						l.Error().Err(err).Msg("error getting retry")
						continue
					}

					if !retry {
						continue
					}

					if err := c.handler.Handle(ctx, message); err != nil {
						l.Error().Err(err).Msg("handler error")
						var reErr *RetryError
						switch {
						case errors.As(err, &reErr):
							if err := setRetryAfter(ctx, c.redis, message.ID.String(), reErr.RetryDelay()); err != nil {
								l.Error().Err(err).Msg("error setting retry after")
							}
							continue
						default:
							if err := c.error.Handle(ctx, xMsgs[i], err); err != nil {
								l.Error().Err(err).Str("x_message_id", xMsgs[i].ID).Msg("error handling error")
								sentry.CaptureException(err)
								continue
							}
						}
					}
					ackC <- xMsgs[i].ID
				}
			}
		}
	}()
	return ackC
}

// ackAsync periodically ack the messages for the consumer.
func (c *consumer) ackAsync(ctx context.Context, ackC <-chan string) {
	go func() {
		l := logging.Logger(ctx, "Consumer.ackAsync")

		cache := make([]string, 0, c.conf.cacheLimit)
		ticker := time.NewTicker(c.conf.cacheTimeout)
		ackFn := func(config []string) error {
			if err := c.redis.XAck(ctx, c.conf.streamName, c.conf.consumerGroup, config...); err != nil {
				return err
			}
			// Empty the cache after we successfully ack the messages.
			cache = cache[:0]
			return nil
		}

		for {
			select {
			case <-ctx.Done():
				return
			case xMessageID := <-ackC:
				cache = append(cache, xMessageID)
				if len(cache) < c.conf.cacheLimit {
					break
				}

				ticker.Stop()

				if err := ackFn(cache); err != nil {
					// log the error and allow it to retry until we max out our retry attempts
					l.Error().Err(err).Strs("message_ids", cache).Msg("batch consumer processing")
					sentry.CaptureException(err)
				}

				ticker = time.NewTicker(c.conf.cacheTimeout)

			case <-ticker.C:
				if len(cache) < 1 {
					break
				}

				if err := ackFn(cache); err != nil {
					l.Error().Err(err).Strs("message_ids", cache).Msg("batch consumer processing")
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
func (c *consumer) statusAsync(ctx context.Context) <-chan error {
	resultC := make(chan error)
	go func() {
		defer close(resultC)

		l := logging.Logger(ctx, "StreamConsumer.statusAsync")

		ticker := time.NewTicker(c.conf.statusTimeout)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				infoGroup, err := c.redis.XInfoGroup(ctx, c.conf.streamName, c.conf.consumerGroup)
				if err != nil {
					l.Error().Err(err).Msg("error getting xinfo groups")
					continue
				}

				// This is a workaround for not being able to check the cg lag (see comment below).
				lastMessage, err := c.redis.GetLastMessage(ctx, c.conf.streamName)
				if err != nil {
					l.Error().Err(err).Msg("error calling xrevrange")
					continue
				}

				// Currently we cannot get the lag, so we need to check the ID of the last entry delivered to the consumer group,
				// if this matches the last xMessageID in the stream we can assume all messages have been delivered.
				if infoGroup.Pending == 0 && infoGroup.LastDeliveredID == lastMessage.ID {
					resultC <- nil
				}
			}
		}
	}()
	return resultC
}

// Config defines the configuration to be used by the StreamConsumer.
type Config struct {
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

// Option func to build StreamConsumerConfig.
type Option func(c *Config)

// NewConfig return a new instance of StreamConsumerConfig.
func NewConfig(options ...Option) *Config {
	c := &Config{
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
		option(c)
	}
	return c
}

// WithStreamName sets the stream name.
func WithStreamName(streamName string) Option {
	return func(c *Config) {
		c.streamName = streamName
	}
}

// WithConsumerID sets the consumer id.
func WithConsumerID(consumerID string) Option {
	return func(c *Config) {
		c.consumerID = consumerID
	}
}

// WithConsumerGroup sets the consumer group.
func WithConsumerGroup(consumerGroup string) Option {
	return func(c *Config) {
		c.consumerGroup = consumerGroup
	}
}

// WithStart sets position to start processing messages. Default 0.
func WithStart(start string) Option {
	return func(c *Config) {
		c.start = start
	}
}

// WithCount sets the maximum number of messages to read from the stream. Default 10.
func WithCount(count int64) Option {
	return func(c *Config) {
		c.count = count
	}
}

// WithBlock sets the time.Duration a consumer should block when the stream is empty. Default 0.
func WithBlock(block time.Duration) Option {
	return func(c *Config) {
		c.block = block
	}
}

// WithMinIdleTime sets the minimum time to wait before a message a pending/failed message
// can be auto claimed for reprocess. Default 5 * time.Second.
func WithMinIdleTime(minIdleTime time.Duration) Option {
	return func(c *Config) {
		c.minIdleTime = minIdleTime
	}
}

// WithMaxRetry sets the maximum number of times a pending message should be retried. Default 5.
// MaxRetryEnabled must be set to true for this to take effect.
func WithMaxRetry(maxRetry int64) Option {
	return func(c *Config) {
		c.maxRetry = maxRetry
	}
}

// WithCacheLimit sets the maximum number of message to batch before calling ack. Default 10.
func WithCacheLimit(cacheLimit int) Option {
	return func(c *Config) {
		c.cacheLimit = cacheLimit
	}
}

// WithCacheTimeout sets the amount of time to wait before calling ack. Default 1 * time.Second.
func WithCacheTimeout(cacheTimeout time.Duration) Option {
	return func(c *Config) {
		c.cacheTimeout = cacheTimeout
	}
}

// WithStatusTimeout sets the amount of time to wait before checking if all messages have been processed for a
// given stream. Default 10 * time.Second.
func WithStatusTimeout(statusTimeout time.Duration) Option {
	return func(c *Config) {
		c.statusTimeout = statusTimeout
	}
}

func (c *Config) String() string {
	return "id=" + c.consumerID + " cg=" + c.consumerGroup + " stream=" + c.streamName
}

// TODO(clD11) Temp, revisit once we finalise retry and transient errors, improve this error type.
//  Consider making the retry after default greater than zero to avoid potential bug of setting redis
//  Set to 0 which means it never expires.

// RetryError represents a transient error which and can be retried.
type RetryError struct {
	RetryAfter time.Duration
	Err        error
}

func NewRetryError(ra time.Duration, err error) error {
	return &RetryError{
		RetryAfter: ra,
		Err:        err,
	}
}

// Unwrap returns the nested error if any, or nil.
func (e *RetryError) Unwrap() error { return e.Err }

// Error returns the error message.
func (e *RetryError) Error() string {
	const m = "retry error"
	if e.Err != nil {
		return e.Err.Error()
	}
	return m
}

// RetryDelay returns the advised time to wait before retrying.
func (e *RetryError) RetryDelay() time.Duration {
	const raDefault = 1 * time.Second
	if e.RetryAfter > 0 {
		return e.RetryAfter * time.Second
	}
	return raDefault
}

func newMessage(x redis.XMessage) (Message, error) {
	d, ok := x.Values[dataKey]
	if !ok {
		return Message{}, errDataKeyNotFound
	}

	s, ok := d.(string)
	if !ok {
		return Message{}, fmt.Errorf("unknow msg data type: %+v", s)
	}

	message, err := NewMessageFromString(s)
	if err != nil {
		return Message{}, err
	}

	if len(message.Body) == 0 {
		return Message{}, errNoMsgBody
	}

	return message, nil
}

func setRetryAfter(ctx context.Context, rc RedisClient, key string, exp time.Duration) error {
	_, err := rc.Set(ctx, redis.SetArgs{
		Key:        retryAfterPrefix + key,
		Expiration: exp,
	})
	if err != nil {
		return err
	}
	return nil
}

// Check to see if there is a retry after value, if no key exists then we can process the message again.
func fetchRetryAfter(ctx context.Context, rc RedisClient, key string) (bool, error) {
	_, err := rc.Get(ctx, retryAfterPrefix+key)
	if err != nil {
		if errors.Is(err, redis.ErrKeyDoesNotExist) {
			return true, nil
		}
		return false, err
	}
	// a value must exist so dont retry
	return false, nil
}

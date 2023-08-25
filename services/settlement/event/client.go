package event

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/services/settlement/lua"
	redis "github.com/go-redis/redis/v8"
	uuid "github.com/satori/go.uuid"
)

const (
	// dialTimeout timeout for dialler.
	dialTimeout = 15 * time.Second
	// readTimeout timeout for socket reads.
	readTimeout = 5 * time.Second
	// writeTimeout timeout for socket writes.
	writeTimeout = 5 * time.Second
	// maxRetries number of retries before giving up.
	maxRetries = 5
	// minRetryBackoff backoff between each retry.
	minRetryBackoff = 5 * time.Millisecond
	// maxRetryBackoff backoff between each retry.
	maxRetryBackoff = 500 * time.Millisecond
	// dataKey is the key used to retrieve the event.Message value from the redis stream message.
	dataKey = "data"
	// XRedisIDKey is the key used to set the redis message is header.
	XRedisIDKey = "x-redis-id"
	// ReleaseLockSuccess is the value returned when a lock has been released successfully.
	ReleaseLockSuccess = 1
)

var (
	// ErrLockValueDoesNotMatch is the error returned when the provided value does not match the value
	// stored at the given key.
	ErrLockValueDoesNotMatch = errors.New("lock value does not match")
)

// RedisClient defines a event client
type RedisClient struct {
	*redis.Client
}

// NewRedisClient creates a new instance of redis client
func NewRedisClient(address string, username, password string) *RedisClient {
	return &RedisClient{redis.NewClient(&redis.Options{
		Addr:            address,
		Username:        username,
		Password:        password,
		DialTimeout:     dialTimeout,
		ReadTimeout:     readTimeout,
		MaxRetries:      maxRetries,
		MinRetryBackoff: minRetryBackoff,
		MaxRetryBackoff: maxRetryBackoff,
		WriteTimeout:    writeTimeout,
		TLSConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			ClientAuth:         0,
			InsecureSkipVerify: true,
		},
	})}
}

// Send wraps the event.Message in dataKey field and sends to stream.
func (r *RedisClient) Send(ctx context.Context, stream string, message *Message) error {
	_, err := r.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]interface{}{dataKey: *message}}).Result()
	if err != nil {
		return fmt.Errorf("redis client: error adding message to redis stream: %w", err)
	}
	return nil
}

// Read reads messages from the given streams and returns the array of messages.
// This function wraps the redis XRead command see redis documentation for details on provided arguments.
// This function also ads the underlying redis message id to the headers with the key XRedisIDKey.
func (r *RedisClient) Read(ctx context.Context, streams []string, count int64, block time.Duration) ([]*Message, error) {
	xStreams, err := r.XRead(ctx, &redis.XReadArgs{
		Streams: streams,
		Count:   count,
		Block:   block,
	}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("error calling xread: %w", err)
	}

	var messages []*Message
	for _, xStream := range xStreams {
		var xMessage redis.XMessage
		for _, xMessage = range xStream.Messages {
			data, ok := xMessage.Values[dataKey]
			if !ok {
				return nil, fmt.Errorf("error data key not found")
			}
			switch s := data.(type) {
			case string:
				message, err := NewMessageFromString(s)
				if err != nil || len(message.Body) == 0 {
					return nil, fmt.Errorf("error creating new message: %w", err)
				}
				message.SetHeader(XRedisIDKey, xMessage.ID)
				messages = append(messages, message)
			default:
				return nil, fmt.Errorf("error unknown data type: %+v", s)
			}
		}
	}
	return messages, nil
}

// AcquireLock acquires the lock for the given key and value. If a zero argument is supplied for the expiration
// time then the lock will not expire and will be held until released.
func (r *RedisClient) AcquireLock(ctx context.Context, key string, value uuid.UUID, expiration time.Duration) (bool, error) {
	_, err := r.SetNX(ctx, key, value, expiration).Result()
	if err != nil {
		return false, fmt.Errorf("error acquiring lock for key %s: %w", key, err)
	}
	return true, nil
}

// ReleaseLock release the lock for the given key and value and returns ReleaseLockSuccess if successful and an
// error otherwise. Release lock performs a check before releasing the lock to avoid releasing a lock held by
// another client. For example, a client may acquire the lock for a given key then take longer than the expiration
// time for the acquired lock and then release a lock which had been acquired by another client in the meantime.
// Both the key and value must match the original acquired lock otherwise a ErrLockValueDoesNotMatch is returned.
func (r *RedisClient) ReleaseLock(ctx context.Context, key string, value uuid.UUID) (int, error) {
	k := []string{key}
	num, err := lua.Unlock.Run(ctx, r, k, value).Int()
	if err != nil {
		return 0, fmt.Errorf("error releasing lock for key %s: %w", key, err)
	}

	if num == 0 {
		return 0, ErrLockValueDoesNotMatch
	}

	if num > 1 {
		return num, fmt.Errorf("error should have released 1 lock got %d", num)
	}

	return ReleaseLockSuccess, nil
}

package event

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/services/settlement/lua"
	"github.com/go-redis/redis/v8"
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
)

// RedisClient defines a event client
type RedisClient struct {
	*redis.ClusterClient
}

// NewRedisClient creates a new instance of redis client
func NewRedisClient(addresses []string, username, password string) (*RedisClient, error) {
	return &RedisClient{redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:           addresses,
		Username:        username,
		Password:        password,
		DialTimeout:     dialTimeout,
		ReadTimeout:     readTimeout,
		MaxRetries:      maxRetries,
		MinRetryBackoff: minRetryBackoff,
		MaxRetryBackoff: maxRetryBackoff,
		WriteTimeout:    writeTimeout,
		RouteByLatency:  true,
		TLSConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			ClientAuth:         0,
			InsecureSkipVerify: true,
		},
	})}, nil
}

// Send wraps the event.Message in dataKey field and sends to stream.
func (r *RedisClient) Send(ctx context.Context, stream string, message *Message) error {
	_, err := r.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]interface{}{dataKey: *message}}).Result()
	if err != nil {
		return fmt.Errorf("redis client: error adding message to redis stream: %w", err)
	}
	return nil
}

func (r *RedisClient) Read(ctx context.Context, args *redis.XReadArgs) ([]*Message, error) {
	xStreams, err := r.XRead(ctx, args).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("error calling xread: %w", err)
	}

	var messages []*Message

	for _, xStream := range xStreams {
		var xMessage redis.XMessage
		for _, xMessage = range xStream.Messages {
			data, ok := xMessage.Values[dataKey]
			if !ok {
				continue
			}
			switch s := data.(type) {
			case string:
				message, err := NewMessageFromString(s)
				if err != nil || len(message.Body) == 0 {
					return nil, fmt.Errorf("error creating new message: %w", err)
				}
				//TODO remove this if not needed
				message.SetHeader(XRedisIDKey, xMessage.ID)
				messages = append(messages, message)
			default:
				return nil, fmt.Errorf("error unknown data type: %+v", s)
			}
		}
	}
	return messages, nil
}

func (r *RedisClient) AcquireLock(ctx context.Context, key string, value uuid.UUID, expiration time.Duration) (bool, error) {
	return r.SetNX(ctx, key, value, expiration).Result()
}

// TODO return error instead of 0
func (r *RedisClient) ReleaseLock(ctx context.Context, key string, value uuid.UUID) (int, error) {
	k := []string{key}
	num, err := lua.Unlock.Run(ctx, r, k, value).Int()
	if err != nil {
		return 0, fmt.Errorf("redis client: error adding message to redis stream: %w", err)
	}
	return num, nil
}

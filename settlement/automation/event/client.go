package event

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	// readTimeout timeout for socket reads
	readTimeout = 5 * time.Second
	// writeTimeout timeout for socket writes
	writeTimeout = 5 * time.Second
	// maxRetries number of retries before giving up
	maxRetries = 5
	// minRetryBackoff backoff between each retry
	minRetryBackoff = 5 * time.Millisecond
	// maxRetryBackoff backoff between each retry
	maxRetryBackoff = 500 * time.Millisecond
	// dataKey this is the key used retrieve the event.Message value from the redis stream message
	dataKey = "data"
)

// Client defines a event client
type Client struct {
	*redis.Client
}

// NewRedisClient creates a new instance of redis client
func NewRedisClient(redisURL string) (*Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redis client: error creating new redis client: %w", err)
	}
	return &Client{
		Client: redis.NewClient(&redis.Options{
			Addr:            opt.Addr,
			Password:        "",
			DB:              0,
			ReadTimeout:     readTimeout,
			MaxRetries:      maxRetries,
			MinRetryBackoff: minRetryBackoff,
			MaxRetryBackoff: maxRetryBackoff,
			WriteTimeout:    writeTimeout,
		}),
	}, nil
}

// Send wraps the event.Message in dataKey field and sends to stream
func (r *Client) Send(ctx context.Context, message Message, stream string) error {
	_, err := r.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]interface{}{dataKey: message}}).Result()
	if err != nil {
		return fmt.Errorf("redis client: error adding message to redis stream: %w", err)
	}
	return nil
}

// SendRawMessage wraps the message in dataKey field and sends to stream
func (r *Client) SendRawMessage(ctx context.Context, message map[string]interface{}, stream string) error {
	_, err := r.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]interface{}{dataKey: message}}).Result()
	if err != nil {
		return fmt.Errorf("redis client: error adding raw message to redis stream: %w", err)
	}
	return nil
}

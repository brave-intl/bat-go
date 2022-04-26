package event

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	// dialTimeout timeout for dialler
	dialTimeout = 15 * time.Second
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
	// dataKey is the key used to retrieve the event.Message value from the redis stream message
	dataKey = "data"
)

// Client defines a event client
type Client struct {
	*redis.ClusterClient
}

// NewRedisClient creates a new instance of redis client
func NewRedisClient(addresses []string, username, password string) (*Client, error) {
	return &Client{redis.NewClusterClient(&redis.ClusterOptions{
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
			InsecureSkipVerify: true,
			ClientAuth:         0,
		},
	})}, nil
}

// Send wraps the event.Message in dataKey field and sends to stream
func (r *Client) Send(ctx context.Context, message Message, stream string) error {
	_, err := r.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]interface{}{dataKey: message}}).Result()
	if err != nil {
		return fmt.Errorf("redis client: error adding message to redis stream: %w", err)
	}
	return nil
}

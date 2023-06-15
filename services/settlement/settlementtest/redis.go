// Package settlementtest provides utilities for testing skus. Do not import this into non-test code.
package settlementtest

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/go-redis/redis/v8"

	"github.com/brave-intl/bat-go/services/settlement/event"
	"github.com/stretchr/testify/require"
)

const (
	PrepareConfig = "prepare-config"
	SubmitConfig  = "submit-config"
)

// StreamsTearDown cleanup redis streams.
func StreamsTearDown(t *testing.T) {
	redisAddress := os.Getenv("REDIS_URL")
	require.NotNil(t, redisAddress)

	redisUsername := os.Getenv("REDIS_USERNAME")
	require.NotNil(t, redisUsername)

	redisPassword := os.Getenv("REDIS_PASSWORD")
	require.NotNil(t, redisPassword)

	redisAddresses := []string{fmt.Sprintf("%s:6379", redisAddress)}
	rc, err := event.NewRedisClient(redisAddresses, redisUsername, redisPassword)
	require.NoError(t, err)

	_, err = rc.Do(context.Background(), "DEL", PrepareConfig).Result()
	require.NoError(t, err)

	_, err = rc.Do(context.Background(), "DEL", SubmitConfig).Result()
	require.NoError(t, err)
}

// NewSuccessHandler returns an instance of a success handler for use in testing.
func NewSuccessHandler() event.Handler {
	return &successHandler{}
}

type successHandler struct{}

func (s *successHandler) Handle(ctx context.Context, message event.Message) error {
	return nil
}

// NewErrorHandler returns a new instance of error handler.
// The attempts fields determines how many times an error should be returned before returning success.
func NewErrorHandler(attempts int, handleError error) event.Handler {
	return &errorHandler{
		attempts:    attempts,
		handleError: handleError,
	}
}

type errorHandler struct {
	attempts      int
	attemptsCount int
	handleError   error
}

// Handle implements a test handler.
func (e *errorHandler) Handle(ctx context.Context, message event.Message) error {
	if e.attemptsCount >= e.attempts {
		return nil
	}
	e.attemptsCount++
	return e.handleError
}

// NewDQLHandler returns an instance of a dlq handler for use in testing.
func NewDQLHandler() event.ErrorHandler {
	return &dqlHandler{}
}

type dqlHandler struct{}

func (d *dqlHandler) Handle(ctx context.Context, xMessage redis.XMessage, processingError error) (err error) {
	return nil
}

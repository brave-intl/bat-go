package test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	testutils "github.com/brave-intl/bat-go/utils/test"
	"github.com/stretchr/testify/require"
)

// StreamsTearDown cleanup redis streams
func StreamsTearDown(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	require.NotNil(t, redisURL)

	redisUsername := os.Getenv("REDIS_USERNAME")
	require.NotNil(t, redisUsername)

	redisPassword := os.Getenv("REDIS_PASSWORD")
	require.NotNil(t, redisPassword)

	rc, err := event.NewRedisClient(redisURL, redisUsername, redisPassword)
	require.NoError(t, err)

	_, err = rc.Do(context.Background(), "DEL", event.PrepareStream).Result()
	require.NoError(t, err)

	_, err = rc.Do(context.Background(), "DEL", event.NotifyStream).Result()
	require.NoError(t, err)

	_, err = rc.Do(context.Background(), "DEL", event.SubmitStream).Result()
	require.NoError(t, err)

	_, err = rc.Do(context.Background(), "DEL", event.SubmitStatusStream).Result()
	require.NoError(t, err)

	_, err = rc.Do(context.Background(), "DEL", event.CheckStatusStream).Result()
	require.NoError(t, err)
}

type channelHandler struct {
	actualC chan event.Message
}

// Handle implements a test handler that writes the received messages a channel
func (c *channelHandler) Handle(ctx context.Context, messages []event.Message) error {
	for _, message := range messages {
		c.actualC <- message
	}
	return nil
}

// StartTestBatchConsumer helper to start a new batch consumer.
// Handled messages are written to provided channel.
func StartTestBatchConsumer(t *testing.T, ctx context.Context, redisClient *event.Client, stream string, // nolint
	actualC chan event.Message) {
	t.Helper()
	StartTestBatchConsumerWithRouter(t, ctx, redisClient, stream, fmt.Sprintf("test-consumer-group-%s",
		testutils.RandomString()), "", nil, actualC)
}

// StartTestBatchConsumerWithRouter helper to start a new batch consumer.
// Handled messages are written to provided channel.
func StartTestBatchConsumerWithRouter(t *testing.T, ctx context.Context, redisClient *event.Client, stream, // nolint
	consumerGroup, DLQ string, router event.Router, actualC chan event.Message) {
	t.Helper()

	consumerConfig, err := event.NewBatchConsumerConfig(event.WithStreamName(stream),
		event.WithConsumerGroup(consumerGroup))
	require.NoError(t, err)

	ch := &channelHandler{
		actualC: actualC,
	}
	consumer, err := event.NewBatchConsumer(redisClient, *consumerConfig, ch, router, DLQ)
	require.NoError(t, err)

	err = consumer.Consume(ctx)
	require.NoError(t, err)
}

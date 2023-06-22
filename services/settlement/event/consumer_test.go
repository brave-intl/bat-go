//go:build integration

package event_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/logging"
	testutils "github.com/brave-intl/bat-go/libs/test"
	"github.com/brave-intl/bat-go/services/settlement/event"
	"github.com/brave-intl/bat-go/services/settlement/settlementtest"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
)

type ConsumerTestSuite struct {
	suite.Suite
	redis         *event.RedisClient
	stream        string
	consumerGroup string
	dlq           string
}

func TestConsumerTestSuite(t *testing.T) {
	suite.Run(t, new(ConsumerTestSuite))
}

func (suite *ConsumerTestSuite) SetupSuite() {
	suite.stream = testutils.RandomString()
	suite.consumerGroup = testutils.RandomString()
	suite.dlq = testutils.RandomString()

	redisAddress := os.Getenv("REDIS_URL")
	suite.NotNil(redisAddress)

	redisUsername := os.Getenv("REDIS_USERNAME")
	suite.Require().NotNil(redisUsername)

	redisPassword := os.Getenv("REDIS_PASSWORD")
	suite.Require().NotNil(redisPassword)

	redisAddresses := []string{fmt.Sprintf("%s:6379", redisAddress)}
	rc := event.NewRedisClient(redisAddresses, redisUsername, redisPassword)

	suite.redis = rc
}

func (suite *ConsumerTestSuite) SetupTest() {
	_, err := suite.redis.Do(context.Background(), "DEL", suite.stream).Result()
	suite.Require().NoError(err)

	_, err = suite.redis.Do(context.Background(), "DEL", suite.dlq).Result()
	suite.Require().NoError(err)
}

func (suite *ConsumerTestSuite) TestConsumer_ProcessAsync_Success() {
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)

	for i := 0; i < 5; i++ {
		message := &event.Message{
			ID:        uuid.NewV4(),
			Timestamp: time.Now(),
			Body:      testutils.RandomString(),
		}
		err := suite.redis.Send(context.Background(), suite.stream, message)
		suite.Require().NoError(err)
	}

	// assert all messages were successfully written to stream
	streamCount, err := suite.redis.XLen(ctx, suite.stream).Result()
	suite.Require().NoError(err)
	suite.Require().Equal(int64(5), streamCount)

	config, err := event.NewBatchConsumerConfig(
		event.WithStreamName(suite.stream),
		event.WithConsumerID("test-process-success"),
		event.WithConsumerGroup(suite.consumerGroup),
		event.WithCacheLimit(5),
		event.WithStatusTimeout(time.Millisecond))
	suite.Require().NoError(err)

	h := settlementtest.NewSuccessHandler()
	c := event.NewBatchConsumer(suite.redis, *config, h, nil)

	resultC := make(chan error)

	err = c.Start(ctx, resultC)
	suite.Require().NoError(err)

	err = <-resultC
	suite.Require().NoError(err)

	// assert all messages have been ack and there are no pending messages for stream or consumer group
	xPending, err := suite.redis.XPending(ctx, suite.stream, suite.consumerGroup).Result()
	suite.Require().NoError(err)
	suite.Equal(int64(0), xPending.Count)

	cancel()
}

func (suite *ConsumerTestSuite) TestConsumer_RetryAsync_Success() {
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)

	message := &event.Message{
		ID:        uuid.NewV4(),
		Timestamp: time.Now(),
		Body:      testutils.RandomString(),
	}
	err := suite.redis.Send(context.Background(), suite.stream, message)
	suite.Require().NoError(err)

	// assert all messages were successfully written to stream
	streamCount, err := suite.redis.XLen(ctx, suite.stream).Result()
	suite.Require().NoError(err)
	suite.Require().Equal(int64(1), streamCount)

	config, err := event.NewBatchConsumerConfig(
		event.WithStreamName(suite.stream),
		event.WithConsumerID("test-process-success"),
		event.WithConsumerGroup(suite.consumerGroup),
		event.WithCacheLimit(1),
		event.WithStatusTimeout(time.Millisecond))
	suite.Require().NoError(err)

	// attempt to send the message 2 times before succeeding
	transientError := event.RetryError{}
	h := settlementtest.NewErrorHandler(2, transientError)
	c := event.NewBatchConsumer(suite.redis, *config, h, nil)

	resultC := make(chan error)

	err = c.Start(ctx, resultC)
	suite.Require().NoError(err)

	err = <-resultC
	suite.Require().NoError(err)

	// assert all messages have been ack and none pending for stream and consumer group
	xPending, err := suite.redis.XPending(ctx, suite.stream, suite.consumerGroup).Result()
	suite.Require().NoError(err)
	suite.Equal(int64(0), xPending.Count)

	cancel()
}

func (suite *ConsumerTestSuite) TestConsumer_ErrorHandler_Success() {
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)

	message := &event.Message{
		ID:        uuid.NewV4(),
		Timestamp: time.Now(),
		Body:      testutils.RandomString(),
	}
	err := suite.redis.Send(context.Background(), suite.stream, message)
	suite.Require().NoError(err)

	// assert all messages were successfully written to stream
	streamCount, err := suite.redis.XLen(ctx, suite.stream).Result()
	suite.Require().NoError(err)
	suite.Require().Equal(int64(1), streamCount)

	config, err := event.NewBatchConsumerConfig(
		event.WithStreamName(suite.stream),
		event.WithConsumerID("test-process-success"),
		event.WithConsumerGroup(suite.consumerGroup),
		event.WithCacheLimit(1),
		event.WithStatusTimeout(time.Millisecond))
	suite.Require().NoError(err)

	// the handler will return a permanent error which results in the message being sent to he dlq handler
	permanentError := errors.New("permanent error")
	h := settlementtest.NewErrorHandler(1, permanentError)
	c := event.NewBatchConsumer(suite.redis, *config, h, settlementtest.NewDQLHandler())

	resultC := make(chan error)

	err = c.Start(ctx, resultC)
	suite.Require().NoError(err)

	err = <-resultC
	suite.Require().NoError(err)

	// assert all messages have been ack and none pending for stream and consumer group
	xPending, err := suite.redis.XPending(ctx, suite.stream, suite.consumerGroup).Result()
	suite.Require().NoError(err)
	suite.Equal(int64(0), xPending.Count)

	cancel()
}

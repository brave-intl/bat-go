//go:build integration

package event_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/settlement/automation/test"
	"github.com/brave-intl/bat-go/utils/logging"
	testutils "github.com/brave-intl/bat-go/utils/test"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
)

type ConsumerTestSuite struct {
	suite.Suite
	stream        string
	consumerGroup string
	DLQ           string
}

func TestConsumerTestSuite(t *testing.T) {
	suite.Run(t, &ConsumerTestSuite{
		stream:        testutils.RandomString(),
		consumerGroup: testutils.RandomString(),
		DLQ:           testutils.RandomString(),
	})
}

func (suite *ConsumerTestSuite) SetupTest() {
	redisAddress := os.Getenv("REDIS_URL")
	suite.NotNil(redisAddress)

	redisUsername := os.Getenv("REDIS_USERNAME")
	suite.Require().NotNil(redisUsername)

	redisPassword := os.Getenv("REDIS_PASSWORD")
	suite.Require().NotNil(redisPassword)

	redisAddresses := []string{fmt.Sprintf("%s:6379", redisAddress)}
	rc, err := event.NewRedisClient(redisAddresses, redisUsername, redisPassword)
	suite.NoError(err)

	_, err = rc.Do(context.Background(), "DEL", suite.stream).Result()
	suite.NoError(err)

	_, err = rc.Do(context.Background(), "DEL", suite.DLQ).Result()
	suite.NoError(err)
}

func (suite *ConsumerTestSuite) TestConsumer_Process_Success() {
	redisAddress := os.Getenv("REDIS_URL")
	suite.Require().NotNil(redisAddress)

	redisUsername := os.Getenv("REDIS_USERNAME")
	suite.Require().NotNil(redisUsername)

	redisPassword := os.Getenv("REDIS_PASSWORD")
	suite.Require().NotNil(redisPassword)

	// create newHandler redis client and clear streams
	redisAddresses := []string{fmt.Sprintf("%s:6379", redisAddress)}
	redis, err := event.NewRedisClient(redisAddresses, redisUsername, redisPassword)
	suite.Require().NoError(err)

	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx, done := context.WithTimeout(ctx, 10*time.Second)

	messages := make(map[uuid.UUID]event.Message)
	for i := 0; i < 5; i++ {
		message := event.Message{
			ID:        uuid.NewV4(),
			Timestamp: time.Now(),
			Type:      event.MessageType(testutils.RandomString()),
			Routing: &event.Routing{
				Position: 0,
				Slip: []event.Step{
					{
						Stream:  testutils.RandomString(),
						OnError: testutils.RandomString(),
					},
				},
			},
			Body: testutils.RandomString(),
		}

		messages[message.ID] = message

		err = redis.Send(context.Background(), message, suite.stream)
		suite.Require().NoError(err)
	}

	router := func(message *event.Message) error {
		return nil
	}

	actualC := make(chan event.Message, len(messages))
	go test.StartTestBatchConsumerWithRouter(suite.T(), ctx, redis,
		suite.stream, suite.consumerGroup, suite.DLQ, router, actualC)

	// assert messages processed match the sent messages
	var actual event.Message
	for i := 0; i < len(messages); i++ {
		actual = <-actualC
		expected := messages[actual.ID]
		suite.Equal(expected.ID, actual.ID)
		suite.Equal(expected.Type, actual.Type)
		suite.WithinDuration(expected.Timestamp, actual.Timestamp, 1*time.Second)
		suite.NotNil(actual.Headers[event.HeaderCorrelationID])
		suite.Equal(expected.Routing, actual.Routing)
		suite.Equal(expected.Body, actual.Body)
	}

	// assert all messages were successfully written to stream
	streamCount, err := redis.XLen(ctx, suite.stream).Result()
	suite.NoError(err)
	suite.Equal(int64(len(messages)), streamCount)

	// assert all messages have been ack and none pending for stream and consumer group
	xPending, err := redis.XPending(ctx, suite.stream, suite.consumerGroup).Result()
	suite.NoError(err)
	suite.Equal(int64(0), xPending.Count)

	// assert the dlq is empty
	DLQCount, err := redis.XLen(ctx, suite.DLQ).Result()
	suite.NoError(err)
	suite.Equal(int64(0), DLQCount)

	done()
}

type errorHandler struct{}

func (e errorHandler) Handle(ctx context.Context, messages []event.Message) error {
	return errors.New("handler error")
}

func (suite *ConsumerTestSuite) TestConsumer_Process_Handler_Error() {
	redisAddress := os.Getenv("REDIS_URL")
	suite.NotNil(redisAddress)

	redisUsername := os.Getenv("REDIS_USERNAME")
	suite.Require().NotNil(redisUsername)

	redisPassword := os.Getenv("REDIS_PASSWORD")
	suite.Require().NotNil(redisPassword)

	// create newHandler redis client and clear streams
	redisAddresses := []string{fmt.Sprintf("%s:6379", redisAddress)}
	redis, err := event.NewRedisClient(redisAddresses, redisUsername, redisPassword)
	suite.Require().NoError(err)

	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx, done := context.WithTimeout(ctx, 10*time.Second)

	messages := make(map[uuid.UUID]event.Message)
	for i := 0; i < 5; i++ {
		message := event.Message{
			ID:        uuid.NewV4(),
			Timestamp: time.Now(),
			Type:      event.MessageType(testutils.RandomString()),
			Routing: &event.Routing{
				Position: 0,
				Slip: []event.Step{
					{
						Stream:  testutils.RandomString(),
						OnError: testutils.RandomString(),
					},
				},
			},
			Body: testutils.RandomString(),
		}

		messages[message.ID] = message

		err = redis.Send(context.Background(), message, suite.stream)
		suite.Require().NoError(err)
	}

	consumerConfig, err := event.NewBatchConsumerConfig(event.WithStreamName(suite.stream),
		event.WithConsumerGroup(suite.consumerGroup), event.WithMinIdleTime(10*time.Millisecond),
		event.WithRetryDelay(1*time.Millisecond),
		event.WithMaxRetry(2))
	suite.Require().NoError(err)

	handler := errorHandler{}

	consumer, err := event.NewBatchConsumer(redis, *consumerConfig, handler, nil, suite.DLQ)
	suite.Require().NoError(err)

	err = consumer.Consume(ctx)
	suite.NoError(err)

	timer := time.Now().Add(10 * time.Second)

	for {
		DLQCount, err := redis.XLen(ctx, suite.DLQ).Result()
		suite.Require().NoError(err)
		// assert failed messages in dlq
		if DLQCount == int64(5) {
			xPending, err := redis.XPending(ctx, suite.stream, suite.consumerGroup).Result()
			suite.Require().NoError(err)
			suite.Equal(int64(0), xPending.Count)

			// assert all messages were successfully written to stream
			streamCount, err := redis.XLen(ctx, suite.stream).Result()
			suite.NoError(err)
			suite.Require().Equal(int64(len(messages)), streamCount)

			break
		}
		if time.Now().After(timer) {
			suite.Fail("test timeout")
			break
		}
	}
	done()
}

func (suite *ConsumerTestSuite) TestConsumer_Process_DataKey_Error() {
	// TODO
}

func (suite *ConsumerTestSuite) TestConsumer_Process_CreateNewMessage_Error() {
	// TODO
}

func (suite *ConsumerTestSuite) TestConsumer_Process_AttachRouter_Success() {
	// TODO
}

func (suite *ConsumerTestSuite) TestConsumer_Process_AttachRouter_Error() {
	// TODO
}

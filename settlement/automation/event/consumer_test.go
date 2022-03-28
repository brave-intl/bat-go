//go:build integration
// +build integration

package event_test

import (
	"context"
	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/settlement/automation/test"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	testutils "github.com/brave-intl/bat-go/utils/test"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
	"os"
	"testing"
	"time"
)

type ConsumerTestSuite struct {
	suite.Suite
	stream        string
	consumerGroup string
	DLQ           string
}

func TestConsumerTestSuite(t *testing.T) {
	suite.Run(t, &ConsumerTestSuite{
		stream:        "test-dlq",
		consumerGroup: "test-consumer-group",
		DLQ:           "test-dlq",
	})
}

func (suite *ConsumerTestSuite) SetupTest() {
	redisURL := os.Getenv("REDIS_URL")
	suite.NotNil(redisURL)

	rc, err := event.NewRedisClient(redisURL)
	suite.NoError(err)

	_, err = rc.Do(context.Background(), "DEL", suite.stream).Result()
	suite.NoError(err)
}

func (suite *ConsumerTestSuite) TestConsumer_Process_Handle_Success() {
	redisURL := os.Getenv("REDIS_URL")
	suite.Require().NotNil(redisURL)

	// create newHandler redis client and clear streams
	redis, err := event.NewRedisClient(redisURL)
	suite.Require().NoError(err)

	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx = context.WithValue(ctx, appctx.RedisSettlementURLCTXKey, redisURL)
	ctx, done := context.WithCancel(ctx)

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
		suite.NoError(err)
	}

	router := func(message *event.Message) error {
		return nil
	}

	actualC := make(chan event.Message, len(messages))
	go test.StartTestBatchConsumerWithRouter(suite.T(), ctx, redis,
		suite.stream, suite.consumerGroup, suite.DLQ, router, actualC)

	// assert the messages have been processed

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

	// assert all messages ack from stream and dlq is empty
	for {
		xPending, _ := redis.XPending(ctx, suite.stream, suite.consumerGroup).Result()

		xLen, err := redis.XLen(ctx, suite.stream).Result()
		suite.NoError(err)

		DLQLen, err := redis.XLen(ctx, suite.DLQ).Result()
		suite.NoError(err)

		if xPending != nil && xPending.Count == 0 && xLen == int64(len(messages)) {
			break
		}

		suite.Equal(0, DLQLen)
	}

	done()
}

func (suite *ConsumerTestSuite) TestConsumer_Process_Handle_Error() {

}

func (suite *ConsumerTestSuite) TestConsumer_Process_Data_Error() {

}

func (suite *ConsumerTestSuite) TestConsumer_Process_CreateNewMessage_Error() {

}

func (suite *ConsumerTestSuite) TestConsumer_Process_AttachRouter_Success() {

}

func (suite *ConsumerTestSuite) TestConsumer_Process_AttachRouter_Error() {

}

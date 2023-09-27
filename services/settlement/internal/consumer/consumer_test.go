package consumer

import (
	"context"
	"os"
	"testing"

	testutils "github.com/brave-intl/bat-go/libs/test"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer/redis"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	os.Exit(m.Run())
}

func TestBatchConsumer_processAsync(t *testing.T) {
	//ctx := context.Background()
	//ctx, _ = logging.SetupLogger(ctx)
	//ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	//
	//for i := 0; i < 5; i++ {
	//	message := &stream.Message{
	//		ID:        uuid.NewV4(),
	//		Timestamp: time.Now(),
	//		Body:      testutils.RandomString(),
	//	}
	//	err := suite.redis.XAdd(context.Background(), stream.XAddArgs{
	//		Stream: suite.stream,
	//		Values: map[string]interface{}{"data": message},
	//	})
	//	suite.Require().NoError(err)
	//}
	//
	//// assert all messages were successfully written to stream
	//streamCount, err := suite.redis.XLen(ctx, suite.stream)
	//suite.Require().NoError(err)
	//suite.Require().Equal(int64(5), streamCount)
	//
	//conf, err := stream.NewStreamConsumerConfig(
	//	stream.WithStreamName(suite.stream),
	//	stream.WithConsumerID("test-process-success"),
	//	stream.WithConsumerGroup(suite.group),
	//	stream.WithCacheLimit(5),
	//	stream.WithStatusTimeout(time.Millisecond))
	//suite.Require().NoError(err)
	//
	//h := settlementtest.NewSuccessHandler()
	//c := stream.NewBatchConsumer(suite.redis, *conf, h, nil)
	//
	//err = c.Start(ctx)
	//suite.Require().NoError(err)
	//
	//// assert all messages have been ack and there are no pending messages for stream or consumer group
	//pending, err := suite.redis.XPending(ctx, &stream.XPendingArgs{
	//	Stream: suite.stream,
	//	Group:  suite.group,
	//	Count:  1,
	//})
	//suite.Require().NoError(err)
	//suite.Len(pending, 0)
	//
	//cancel()
}

func TestBatchConsumer_retryAsync(t *testing.T) {
	//ctx := context.Background()
	//ctx, _ = logging.SetupLogger(ctx)
	//ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	//
	//message := &stream.Message{
	//	ID:        uuid.NewV4(),
	//	Timestamp: time.Now(),
	//	Body:      testutils.RandomString(),
	//}
	//err := suite.redis.XAdd(context.Background(), stream.XAddArgs{
	//	Stream: suite.stream,
	//	Values: map[string]interface{}{"data": message},
	//})
	//suite.Require().NoError(err)
	//
	//// assert all messages were successfully written to stream
	//streamCount, err := suite.redis.XLen(ctx, suite.stream)
	//suite.Require().NoError(err)
	//suite.Require().Equal(int64(1), streamCount)
	//
	//conf, err := stream.NewStreamConsumerConfig(
	//	stream.WithStreamName(suite.stream),
	//	stream.WithConsumerID("test-process-success"),
	//	stream.WithConsumerGroup(suite.group),
	//	stream.WithCacheLimit(1),
	//	stream.WithStatusTimeout(time.Millisecond))
	//suite.Require().NoError(err)
	//
	//// attempt to send the message 2 times before succeeding
	//transientError := stream.RetryError{}
	//h := settlementtest.NewErrorHandler(2, transientError)
	//c := stream.NewBatchConsumer(suite.redis, *conf, h, nil)
	//
	//err = c.Start(ctx)
	//suite.Require().NoError(err)
	//
	//// assert all messages have been ack and none pending for stream and consumer group
	//pending, err := suite.redis.XPending(ctx, &stream.XPendingArgs{
	//	Stream: suite.stream,
	//	Group:  suite.group,
	//	Count:  1,
	//})
	//suite.Require().NoError(err)
	//suite.Len(pending, 0)
	//
	//cancel()
}

func TestBatchConsumer_ackAsync(t *testing.T) {

}

func TestBatchConsumer_statusAsync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	type fields struct {
		streamClient *mockRedisStreamClient
		conf         *Config
	}

	type args struct {
		ctx context.Context
	}

	type want struct {
		err error
	}

	type test struct {
		name   string
		fields fields
		args   args
		want   want
	}

	tests := []test{
		{
			name: "success",
			fields: fields{
				conf: &Config{
					streamName:    testutils.RandomString(),
					consumerGroup: testutils.RandomString(),
					statusTimeout: 1,
				},
				streamClient: &mockRedisStreamClient{
					fnXInfoGroup: func(ctx context.Context, stream, group string) (redis.XInfoGroup, error) {
						return redis.XInfoGroup{
							Pending:         0,
							LastDeliveredID: "1",
						}, nil
					},
					fnGetLastMessage: func(ctx context.Context, stream string) (redis.XMessage, error) {
						return redis.XMessage{
							ID: "1",
						}, nil
					},
				},
			},
			args: args{
				ctx: ctx,
			},
			want: want{err: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &consumer{
				redis: tt.fields.streamClient,
				conf:  tt.fields.conf,
			}

			resultC := b.statusAsync(tt.args.ctx)

			got := <-resultC

			assert.Equalf(t, tt.want.err, got, "want %v got %v", tt.want.err, got)
		})
	}
}

type mockRedisStreamClient struct {
	fnXInfoGroup     func(ctx context.Context, stream, group string) (redis.XInfoGroup, error)
	fnGetLastMessage func(ctx context.Context, stream string) (redis.XMessage, error)
}

func (m *mockRedisStreamClient) XGroupCreateMKStream(ctx context.Context, stream, group, start string) error {
	return nil
}

func (m *mockRedisStreamClient) XReadGroup(ctx context.Context, args *redis.XReadGroupArgs) ([]redis.XMessage, error) {
	return nil, nil
}

func (m *mockRedisStreamClient) Set(ctx context.Context, args redis.SetArgs) (string, error) {
	return "", nil
}

func (m *mockRedisStreamClient) Get(ctx context.Context, key string) (string, error) {
	return "", nil
}

func (m *mockRedisStreamClient) XPending(ctx context.Context, args *redis.XPendingArgs) ([]redis.XPendingEntry, error) {
	return nil, nil
}

func (m *mockRedisStreamClient) XClaim(ctx context.Context, args redis.XClaimArgs) ([]redis.XMessage, error) {
	return nil, nil
}

func (m *mockRedisStreamClient) XAck(ctx context.Context, stream string, group string, ids ...string) error {
	return nil
}

func (m *mockRedisStreamClient) XInfoGroup(ctx context.Context, stream, group string) (redis.XInfoGroup, error) {
	if m.fnXInfoGroup == nil {
		return redis.XInfoGroup{}, nil
	}
	return m.fnXInfoGroup(ctx, stream, group)
}

func (m *mockRedisStreamClient) GetLastMessage(ctx context.Context, stream string) (redis.XMessage, error) {
	if m.fnGetLastMessage == nil {
		return redis.XMessage{}, nil
	}
	return m.fnGetLastMessage(ctx, stream)
}

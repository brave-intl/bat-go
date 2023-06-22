package factory

import (
	"fmt"

	"github.com/brave-intl/bat-go/services/settlement/payout"
	"github.com/brave-intl/bat-go/services/settlement/prepare/internal/prepare/handler"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/services/settlement/event"
)

const (
	preparePrefix = "prepare-"
	dlqSuffix     = "-dql"
)

type PrepareConsumer struct {
	redisClient   *event.RedisClient
	streamClient  *payout.RedisConfigStreamClient
	paymentClient handler.PaymentClient
}

func NewPrepareConsumer(redisClient *event.RedisClient, streamClient *payout.RedisConfigStreamClient, paymentClient handler.PaymentClient) *PrepareConsumer {
	return &PrepareConsumer{
		redisClient:   redisClient,
		streamClient:  streamClient,
		paymentClient: paymentClient,
	}
}

func (p PrepareConsumer) CreateConsumer(config payout.Config) (event.Consumer, error) {
	bc, err := event.NewBatchConsumerConfig(
		event.WithStreamName(config.Stream),
		event.WithConsumerID(preparePrefix+uuid.NewV4().String()),
		event.WithConsumerGroup(config.ConsumerGroup))
	if err != nil {
		return nil, fmt.Errorf("prepare consumer: error creating new batch consumer config: %w", err)
	}

	prepare := handler.NewHandler(p.streamClient, p.paymentClient, config, backoff.Retry)
	dlq := event.NewDLQErrorHandler(p.redisClient, *bc, config.Stream+dlqSuffix)
	consumer := event.NewBatchConsumer(p.redisClient, *bc, prepare, dlq)

	return consumer, nil
}

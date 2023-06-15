package factory

import (
	"fmt"

	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/libs/clients/payment"
	"github.com/brave-intl/bat-go/services/settlement/event"
	"github.com/brave-intl/bat-go/services/settlement/payout"
	"github.com/brave-intl/bat-go/services/settlement/submit/internal/handler"
	uuid "github.com/satori/go.uuid"
)

const (
	submitPrefix = "submit-"
	dlqSuffix    = "-dql"
)

type ConsumerFactoryFunc func(redis *event.RedisClient, paymentClient payment.Client, config payout.Config) (event.Consumer, error)

func (c ConsumerFactoryFunc) CreateConsumer(redis *event.RedisClient, paymentClient payment.Client, config payout.Config) (event.Consumer, error) {
	bc, err := event.NewBatchConsumerConfig(
		event.WithStreamName(config.Stream),
		event.WithConsumerID(submitPrefix+uuid.NewV4().String()),
		event.WithConsumerGroup(config.ConsumerGroup))
	if err != nil {
		return nil, fmt.Errorf("submit consumer: error creating new batch consumer config: %w", err)
	}

	submit := handler.NewHandler(redis, paymentClient, backoff.Retry)
	dlq := event.NewDLQErrorHandler(redis, *bc, config.Stream+dlqSuffix)
	consumer := event.NewBatchConsumer(redis, *bc, submit, dlq)

	return consumer, nil
}

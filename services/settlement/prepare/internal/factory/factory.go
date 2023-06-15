package factory

import (
	"fmt"

	"github.com/brave-intl/bat-go/libs/clients/payment"
	"github.com/brave-intl/bat-go/services/settlement/payout"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/services/settlement/event"
	"github.com/brave-intl/bat-go/services/settlement/prepare/internal/handler"
)

const (
	preparePrefix = "prepare-"
	dlqSuffix     = "-dql"
)

type ConsumerFactoryFunc func(redis *event.RedisClient, prometheus payment.Client, config payout.Config) (event.Consumer, error)

func (c ConsumerFactoryFunc) CreateConsumer(redis *event.RedisClient, paymentClient payment.Client, config payout.Config) (event.Consumer, error) {
	bc, err := event.NewBatchConsumerConfig(
		event.WithStreamName(config.Stream),
		event.WithConsumerID(preparePrefix+uuid.NewV4().String()),
		event.WithConsumerGroup(config.ConsumerGroup))
	if err != nil {
		return nil, fmt.Errorf("prepare consumer: error creating new batch consumer config: %w", err)
	}

	prepare := handler.NewHandler(redis, paymentClient, config, backoff.Retry)
	dlq := event.NewDLQErrorHandler(redis, *bc, config.Stream+dlqSuffix)
	consumer := event.NewBatchConsumer(redis, *bc, prepare, dlq)

	return consumer, nil
}

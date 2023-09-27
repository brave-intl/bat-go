package prepare

import (
	"context"

	"github.com/brave-intl/bat-go/services/settlement/internal/consumer"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer/redis"
	"github.com/brave-intl/bat-go/services/settlement/internal/payment"
	"github.com/brave-intl/bat-go/services/settlement/prepare/internal/prepare/handler"
	"github.com/brave-intl/bat-go/services/settlement/prepare/internal/prepare/storage"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/libs/backoff"
)

const (
	preparePrefix = "prepare-"
	dlqSuffix     = "-dql"
)

type PaymentClient interface {
	Prepare(ctx context.Context, details payment.SerializedDetails) (payment.AttestedDetails, error)
}

type ConsumerFactory struct {
	rc *redis.Client
	pc PaymentClient
}

func NewFactory(redisClient *redis.Client, paymentClient PaymentClient) *ConsumerFactory {
	return &ConsumerFactory{
		rc: redisClient,
		pc: paymentClient,
	}
}

func (f *ConsumerFactory) CreateConsumer(config payment.Config) (consumer.Consumer, error) {
	bc := consumer.NewConfig(
		consumer.WithStreamName(config.Stream),
		consumer.WithConsumerID(preparePrefix+uuid.NewV4().String()),
		consumer.WithConsumerGroup(config.ConsumerGroup))

	s := storage.NewTransactionStore(f.rc)
	p := handler.New(s, f.pc, config, backoff.Retry)
	d := consumer.NewDLQHandler(f.rc, *bc, config.Stream+dlqSuffix, consumer.NewMessage)
	c := consumer.New(f.rc, bc, p, d)

	return c, nil
}

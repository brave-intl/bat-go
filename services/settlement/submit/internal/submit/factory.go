package submit

import (
	"context"

	"github.com/brave-intl/bat-go/services/settlement/internal/consumer"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer/redis"
	"github.com/brave-intl/bat-go/services/settlement/internal/payment"
	"github.com/brave-intl/bat-go/services/settlement/submit/internal/submit/handler"
	uuid "github.com/satori/go.uuid"
)

const (
	submitPrefix = "submit-"
	dlqSuffix    = "-dql"
)

type PaymentClient interface {
	Submit(ctx context.Context, authorizationHeader payment.AuthorizationHeader, details payment.SerializedDetails) (payment.Submit, error)
}

type ConsumerFactory struct {
	rc *redis.Client
	pc PaymentClient
}

func NewFactory(rc *redis.Client, pc PaymentClient) *ConsumerFactory {
	return &ConsumerFactory{
		rc: rc,
		pc: pc,
	}
}

func (f *ConsumerFactory) CreateConsumer(config payment.Config) (consumer.Consumer, error) {
	bc := consumer.NewConfig(
		consumer.WithStreamName(config.Stream),
		consumer.WithConsumerID(submitPrefix+uuid.NewV4().String()),
		consumer.WithConsumerGroup(config.ConsumerGroup))

	s := handler.New(f.pc)
	d := consumer.NewDLQHandler(f.rc, *bc, config.Stream+dlqSuffix, consumer.NewMessage)
	c := consumer.New(f.rc, bc, s, d)

	return c, nil
}

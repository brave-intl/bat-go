package status

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/settlement/automation/custodian"
	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/utils/backoff"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	appctx "github.com/brave-intl/bat-go/utils/context"
	uuid "github.com/satori/go.uuid"
)

func StartConsumer(ctx context.Context) error {
	redisURL := ctx.Value(appctx.RedisSettlementURLCTXKey).(string)
	paymentURL := ctx.Value(appctx.PaymentServiceURLCTXKey).(string)

	consumerConfig, err := event.NewBatchConsumerConfig(
		event.WithStreamName(event.CheckStatusStream),
		event.WithConsumerID(uuid.NewV4()),
		event.WithConsumerGroup(event.CheckStatusConsumerGroup))
	if err != nil {
		return fmt.Errorf("start status consumer: error creating batch consumer config: %w", err)
	}

	redis, err := event.NewRedisClient(redisURL)
	if err != nil {
		return fmt.Errorf("start status consumer: error creating redis client: %w", err)
	}

	handler := newHandler(redis, payment.New(paymentURL), backoff.Retry, custodian.CheckCustodianStatusResponse)

	consumer, err := event.NewBatchConsumer(redis, *consumerConfig, handler, statusRouter, event.DeadLetterQueue)
	if err != nil {
		return fmt.Errorf("start status consumer: error creating new batch consumer: %w", err)
	}

	// start consumer
	err = consumer.Consume(ctx)
	if err != nil {
		return fmt.Errorf("start status consumer: error starting prespare consumer: %w", err)
	}

	return nil
}

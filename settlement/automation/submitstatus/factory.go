package submitstatus

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/settlement/automation/custodian"
	"github.com/brave-intl/bat-go/utils/clients/payment"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/utils/backoff"
	appctx "github.com/brave-intl/bat-go/utils/context"
	uuid "github.com/satori/go.uuid"
)

// StartConsumer initializes and starts the consumer
func StartConsumer(ctx context.Context) error {
	redisURL := ctx.Value(appctx.RedisSettlementURLCTXKey).(string)
	paymentURL := ctx.Value(appctx.PaymentServiceURLCTXKey).(string)

	consumerConfig, err := event.NewBatchConsumerConfig(
		event.WithStreamName(event.SubmitStatusStream),
		event.WithConsumerID(uuid.NewV4()),
		event.WithConsumerGroup(event.SubmitStatusConsumerGroup))
	if err != nil {
		return fmt.Errorf("start submit status consumer: error creating batch consumer config: %w", err)
	}

	redis, err := event.NewRedisClient(redisURL)
	if err != nil {
		return fmt.Errorf("start submit status consumer: error creating redis client: %w", err)
	}

	handler := newHandler(redis, payment.New(paymentURL), backoff.Retry, custodian.CheckCustodianSubmitResponse)

	consumer, err := event.NewBatchConsumer(redis, *consumerConfig, handler, submitStatusRouter, event.DeadLetterQueue)
	if err != nil {
		return fmt.Errorf("start submit status consumer: error creating new batch consumer: %w", err)
	}

	// start the consumer
	err = consumer.Consume(ctx)
	if err != nil {
		return fmt.Errorf("start submit status consumer: error starting submit submit status consumer: %w", err)
	}

	return nil
}

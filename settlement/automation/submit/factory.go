package submit

import (
	"context"
	"crypto"
	"fmt"

	"github.com/brave-intl/bat-go/utils/httpsignature"
	"golang.org/x/crypto/ed25519"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/utils/backoff"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	appctx "github.com/brave-intl/bat-go/utils/context"
	uuid "github.com/satori/go.uuid"
)

// StartConsumer initializes and starts the consumer
func StartConsumer(ctx context.Context) error {
	redisURL := ctx.Value(appctx.RedisSettlementURLCTXKey).(string)
	paymentURL := ctx.Value(appctx.PaymentServiceURLCTXKey).(string)
	httpSigningKey := ctx.Value(appctx.PaymentServiceHTTPSingingCTXKey).(string)

	consumerConfig, err := event.NewBatchConsumerConfig(
		event.WithStreamName(event.SubmitStream),
		event.WithConsumerID(uuid.NewV4()),
		event.WithConsumerGroup(event.SubmitConsumerGroup))
	if err != nil {
		return fmt.Errorf("start submit consumer: error creating batch consumer config: %w", err)
	}

	redis, err := event.NewRedisClient(redisURL)
	if err != nil {
		return fmt.Errorf("start submit consumer: error creating redis client: %w", err)
	}

	pc := httpsignature.ParameterizedSignator{
		SignatureParams: httpsignature.SignatureParams{
			KeyID:     uuid.NewV4().String(),
			Algorithm: httpsignature.ED25519,
			Headers:   []string{"digest", "(request-target)"},
		},
		Signator: ed25519.PrivateKey(httpSigningKey),
		Opts:     crypto.Hash(0),
	}

	handler := newHandler(redis, payment.New(paymentURL, pc), backoff.Retry)

	consumer, err := event.NewBatchConsumer(redis, *consumerConfig, handler, submitRouter, event.DeadLetterQueue)
	if err != nil {
		return fmt.Errorf("start submit consumer: error creating new batch consumer: %w", err)
	}

	// start the consumer
	err = consumer.Consume(ctx)
	if err != nil {
		return fmt.Errorf("start submit consumer: error starting submit consumer: %w", err)
	}

	return nil
}

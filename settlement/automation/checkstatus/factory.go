package checkstatus

import (
	"context"
	"crypto"
	"fmt"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/utils/backoff"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/crypto/ed25519"
)

// StartConsumer initializes and starts the check status consumer
func StartConsumer(ctx context.Context) error {
	redisURL := ctx.Value(appctx.RedisSettlementURLCTXKey).(string)
	paymentURL := ctx.Value(appctx.PaymentServiceURLCTXKey).(string)
	httpSigningKey := ctx.Value(appctx.PaymentServiceHTTPSingingCTXKey).(string)

	consumerConfig, err := event.NewBatchConsumerConfig(
		event.WithStreamName(event.CheckStatusStream),
		event.WithConsumerID(uuid.NewV4()),
		event.WithConsumerGroup(event.CheckStatusConsumerGroup))
	if err != nil {
		return fmt.Errorf("start check status consumer: error creating batch consumer config: %w", err)
	}

	redis, err := event.NewRedisClient(redisURL)
	if err != nil {
		return fmt.Errorf("start check status consumer: error creating redis client: %w", err)
	}

	ps := httpsignature.ParameterizedSignator{
		SignatureParams: httpsignature.SignatureParams{
			KeyID:     uuid.NewV4().String(),
			Algorithm: httpsignature.ED25519,
			Headers:   []string{"digest", "(request-target)"},
		},
		Signator: ed25519.PrivateKey(httpSigningKey),
		Opts:     crypto.Hash(0),
	}

	handler := newHandler(redis, payment.New(paymentURL, ps), backoff.Retry, checkCustodianStatusResponse)

	consumer, err := event.NewBatchConsumer(redis, *consumerConfig, handler, checkStatusRouter, event.DeadLetterQueue)
	if err != nil {
		return fmt.Errorf("start check status consumer: error creating new batch consumer: %w", err)
	}

	// start consumer
	err = consumer.Consume(ctx)
	if err != nil {
		return fmt.Errorf("start check status consumer: error starting prespare consumer: %w", err)
	}

	return nil
}

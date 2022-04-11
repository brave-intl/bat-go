package errored

import (
	"context"
	"crypto"
	"encoding/hex"
	"fmt"
	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/utils/backoff"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/crypto/ed25519"
)

// StartConsumer initializes and start errored consumer
func StartConsumer(ctx context.Context) error {
	redisURL := ctx.Value(appctx.SettlementRedisAddressCTXKey).(string)
	paymentURL := ctx.Value(appctx.PaymentServiceURLCTXKey).(string)
	httpSigningKeyHex := ctx.Value(appctx.PaymentServiceHTTPSingingKeyHexCTXKey).(string)

	consumerConfig, err := event.NewBatchConsumerConfig(
		event.WithStreamName(event.NotifyStream),
		event.WithConsumerID(uuid.NewV4()),
		event.WithConsumerGroup(event.NotifyConsumerGroup))
	if err != nil {
		return fmt.Errorf("start errored consumer: error creating batch consumer config: %w", err)
	}

	redis, err := event.NewRedisClient(redisURL)
	if err != nil {
		return fmt.Errorf("start errored consumer: error creating redis client: %w", err)
	}

	privateKey, err := hex.DecodeString(httpSigningKeyHex)
	if err != nil {
		return fmt.Errorf("start errored consumer: error decoding payment service signing key: %w", err)
	}

	ps := httpsignature.ParameterizedSignator{
		SignatureParams: httpsignature.SignatureParams{
			KeyID:     uuid.NewV4().String(),
			Algorithm: httpsignature.ED25519,
			Headers:   []string{"digest", "(request-target)"},
		},
		Signator: ed25519.PrivateKey(privateKey),
		Opts:     crypto.Hash(0),
	}

	handler := newHandler(redis, payment.New(paymentURL, ps), backoff.Retry)

	consumer, err := event.NewBatchConsumer(redis, *consumerConfig, handler, erroredRouter, event.DeadLetterQueue)
	if err != nil {
		return fmt.Errorf("start errored consumer: error creating new batch consumer: %w", err)
	}

	// start the consumer
	err = consumer.Consume(ctx)
	if err != nil {
		return fmt.Errorf("start errored consumer: error starting consumer: %w", err)
	}

	return nil
}

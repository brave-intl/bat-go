package prepare

import (
	"context"
	"crypto"
	"encoding/hex"
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
	redisAddress := ctx.Value(appctx.SettlementRedisAddressCTXKey).(string)
	redisUsername := ctx.Value(appctx.SettlementRedisUsernameCTXKey).(string)
	redisPassword := ctx.Value(appctx.SettlementRedisPasswordCTXKey).(string)
	paymentURL := ctx.Value(appctx.PaymentServiceURLCTXKey).(string)
	httpSigningKeyHex := ctx.Value(appctx.PaymentServiceHTTPSingingKeyHexCTXKey).(string)

	consumerConfig, err := event.NewBatchConsumerConfig(
		event.WithStreamName(event.PrepareStream),
		event.WithConsumerID(uuid.NewV4()),
		event.WithConsumerGroup(event.PrepareConsumerGroup))
	if err != nil {
		return fmt.Errorf("start prepare consumer: error creating batch consumer config: %w", err)
	}

	redisAddresses := []string{fmt.Sprintf("%s:6379", redisAddress)}
	redis, err := event.NewRedisClient(redisAddresses, redisUsername, redisPassword)
	if err != nil {
		return fmt.Errorf("start prepare consumer: error creating redis client: %w", err)
	}

	privateKey, err := hex.DecodeString(httpSigningKeyHex)
	if err != nil {
		return fmt.Errorf("start prepare consumer: error decoding payment service signing key: %w", err)
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

	consumer, err := event.NewBatchConsumer(redis, *consumerConfig, handler, prepareRouter, event.DeadLetterQueue)
	if err != nil {
		return fmt.Errorf("start prepare consumer: error creating new batch consumer: %w", err)
	}

	// start the consumer
	err = consumer.Consume(ctx)
	if err != nil {
		return fmt.Errorf("start prepare consumer: error starting prepare consumer: %w", err)
	}

	return nil
}

package internal

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/libs/clients/payment"
	appctx "github.com/brave-intl/bat-go/libs/context"
	loggingutils "github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/settlement/event"
	"github.com/brave-intl/bat-go/services/settlement/payout"
	"github.com/brave-intl/bat-go/services/settlement/submit/internal/factory"
)

const (
	// submitConfigStream is the name of the stream where config pertaining to the submit phase of the payout
	// can be retrieved.
	submitConfigStream = "submit-config"
)

type ConsumerFactory interface {
	CreateConsumer(redis *event.RedisClient, paymentClient payment.Client, config payout.Config) (event.Consumer, error)
}

type ConfigStreamAPI interface {
	ReadPayoutConfig(ctx context.Context) (*payout.Config, error)
	SetLastPayout(ctx context.Context, config payout.Config) error
}

// SubmitWorker defines a Submit worker and its dependencies.
type SubmitWorker struct {
	redis           *event.RedisClient
	paymentClient   payment.Client
	consumerFactory ConsumerFactory
	configStream    ConfigStreamAPI
}

// NewSubmitWorker creates a new instance of SubmitWorker.
func NewSubmitWorker(ctx context.Context) (*SubmitWorker, error) {
	redisAddress, ok := ctx.Value(appctx.SettlementRedisAddressCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("new submit worker: error retrieving redis address")
	}

	redisUsername, ok := ctx.Value(appctx.SettlementRedisUsernameCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("new submit worker: error retrieving redis username")
	}

	redisPassword, ok := ctx.Value(appctx.SettlementRedisPasswordCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("new submit worker: error retrieving redis password")
	}

	redisAddresses := []string{fmt.Sprintf("%s:6379", redisAddress)}
	redis, err := event.NewRedisClient(redisAddresses, redisUsername, redisPassword)
	if err != nil {
		return nil, fmt.Errorf("new submit worker: error creating redis client: %w", err)
	}

	paymentURL, ok := ctx.Value(appctx.PaymentServiceURLCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("new submit consumer: error retrieving payment url")
	}
	paymentClient := payment.New(paymentURL)

	csc := payout.NewRedisConfigStreamClient(redis, submitConfigStream)

	return &SubmitWorker{
		redis:           redis,
		paymentClient:   paymentClient,
		consumerFactory: new(factory.ConsumerFactoryFunc),
		configStream:    csc,
	}, nil
}

// Run starts a SubmitWorker. Messages will be consumed from the defined SubmitConfig and passed to
// the SubmitWorker's handler.ConfigHandler for processing.
func (s *SubmitWorker) Run(ctx context.Context) {
	logger := loggingutils.Logger(ctx, "SubmitWorker.Run")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			sc, err := s.configStream.ReadPayoutConfig(ctx)
			if err != nil {
				logger.Error().Err(err).Msg("error reading config")
				continue
			}

			var config payout.Config
			if sc != nil {
				config = *sc
			}

			logger.Info().Interface("submit_config", config).
				Msg("processing submit")

			c, err := s.consumerFactory.CreateConsumer(s.redis, s.paymentClient, config)
			if err != nil {
				logger.Error().Err(err).Msg("error creating consumer")
				continue
			}

			err = consume(ctx, c)
			if err != nil {
				logger.Error().Err(err).Msg("error processing payout")
				continue
			}

			err = s.configStream.SetLastPayout(ctx, config)
			if err != nil {
				logger.Error().Err(err).Msg("error setting last payout")
				continue
			}

			logger.Info().Msg("submit complete")

			//TODO update the payout as complete
		}
	}
}

func consume(ctx context.Context, consumer event.Consumer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel() // calling cancel stops the consumer
	}()

	resultC := make(chan error)
	err := consumer.Start(ctx, resultC)
	if err != nil {
		return fmt.Errorf("error start prepare consumer: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err = <-resultC:
		return err
	}
}

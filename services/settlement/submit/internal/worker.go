package internal

import (
	"context"
	"errors"
	"fmt"

	"github.com/brave-intl/bat-go/libs/clients/payment"
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
func NewSubmitWorker(redisClient *event.RedisClient, paymentClient payment.Client, consumerFactory ConsumerFactory,
	configStream ConfigStreamAPI) *SubmitWorker {
	return &SubmitWorker{
		redis:           redisClient,
		paymentClient:   paymentClient,
		consumerFactory: consumerFactory,
		configStream:    configStream,
	}
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

			//TODO update the payout as complete

			logger.Info().Msg("submit complete")
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

// SubmitConfig hold the configuration for a SubmitWorker.
type SubmitConfig struct {
	redisAddress  string
	redisUsername string
	redisPassword string
	paymentURL    string
	configStream  string
}

// NewSubmitConfig creates and instance of SubmitConfig with the given options.
func NewSubmitConfig(options ...Option) (*SubmitConfig, error) {
	c := &SubmitConfig{
		configStream: submitConfigStream,
	}
	for _, option := range options {
		if err := option(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

type Option func(worker *SubmitConfig) error

// WithRedisAddress sets the redis address.
func WithRedisAddress(address string) Option {
	return func(c *SubmitConfig) error {
		if address == "" {
			return errors.New("redis address cannot be empty")
		}
		c.redisAddress = address
		return nil
	}
}

// WithRedisUsername sets the redis username.
func WithRedisUsername(username string) Option {
	return func(c *SubmitConfig) error {
		if username == "" {
			return errors.New("redis username cannot be empty")
		}
		c.redisUsername = username
		return nil
	}
}

// WithRedisPassword set the redis password.
func WithRedisPassword(password string) Option {
	return func(c *SubmitConfig) error {
		if password == "" {
			return errors.New("redis password cannot be empty")
		}
		c.redisPassword = password
		return nil
	}
}

// WithPaymentClient sets the url for the payment service.
func WithPaymentClient(url string) Option {
	return func(c *SubmitConfig) error {
		if url == "" {
			return errors.New("payment url cannot be empty")
		}
		c.paymentURL = url
		return nil
	}
}

// CreateSubmitWorker is a factory method to create a new instance of SubmitWorker.
func CreateSubmitWorker(config *SubmitConfig) *SubmitWorker {
	redisAddresses := []string{config.redisAddress + ":6379"}
	redisClient := event.NewRedisClient(redisAddresses, config.redisUsername, config.redisPassword)

	paymentClient := payment.New(config.paymentURL)

	configStreamClient := payout.NewRedisConfigStreamClient(redisClient, config.configStream)

	worker := NewSubmitWorker(redisClient, paymentClient,
		new(factory.ConsumerFactoryFunc), configStreamClient)

	return worker
}

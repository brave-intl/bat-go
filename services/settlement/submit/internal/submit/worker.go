package submit

import (
	"context"
	"errors"
	"fmt"

	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer/redis"
	"github.com/brave-intl/bat-go/services/settlement/internal/payment"
)

// submitConfigStream is the name of the stream where config pertaining to the submit phase of the payout can be retrieved.
const submitConfigStream = "submit-config"

type Factory interface {
	CreateConsumer(config payment.Config) (consumer.Consumer, error)
}

type PayoutConfigClient interface {
	ReadPayoutConfig(ctx context.Context) (payment.Config, error)
	SetLastProcessedPayout(ctx context.Context, config payment.Config) error
}

type Worker struct {
	payoutClient PayoutConfigClient
	factory      Factory
}

// NewWorker creates a new instance of submit.Worker.
func NewWorker(payoutClient PayoutConfigClient, factory Factory) *Worker {
	return &Worker{
		payoutClient: payoutClient,
		factory:      factory,
	}
}

// Run starts the Submit transaction flow. This includes submitting the prepared or attested transactions and updating
// the payout as complete once all the transactions have been successfully submitted. Payouts are processed
// sequentially as they are added to the submit payout config stream.
func (w *Worker) Run(ctx context.Context) {
	l := logging.Logger(ctx, "Worker.Run")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			pc, err := w.payoutClient.ReadPayoutConfig(ctx)
			if err != nil {
				l.Error().Err(err).Msg("error reading config")
				continue
			}

			l.Info().Interface("submit_config", pc).Msg("processing submit")

			c, err := w.factory.CreateConsumer(pc)
			if err != nil {
				l.Error().Err(err).Msg("error creating consumer")
				continue
			}

			if err := c.Consume(ctx); err != nil {
				l.Error().Err(err).Msg("error processing payout")
				continue
			}

			if err := w.payoutClient.SetLastProcessedPayout(ctx, pc); err != nil {
				l.Error().Err(err).Msg("error setting last payout")
				continue
			}

			//TODO update the payout as complete
			l.Info().Interface("submit_config", pc).Msg("submit complete")
		}
	}
}

// WorkerConfig holds the configuration for a Worker.
type WorkerConfig struct {
	redisAddress  string
	redisUsername string
	redisPassword string
	paymentURL    string
	configStream  string
}

// NewWorkerConfig creates and instance of WorkerConfig with the given options.
func NewWorkerConfig(opts ...Option) (*WorkerConfig, error) {
	c := &WorkerConfig{
		configStream: submitConfigStream,
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

type Option func(c *WorkerConfig) error

// WithRedisAddress sets the redis address.
func WithRedisAddress(addr string) Option {
	return func(c *WorkerConfig) error {
		if addr == "" {
			return errors.New("redis address cannot be empty")
		}
		c.redisAddress = addr
		return nil
	}
}

// WithRedisUsername sets the redis username.
func WithRedisUsername(username string) Option {
	return func(c *WorkerConfig) error {
		if username == "" {
			return errors.New("redis username cannot be empty")
		}
		c.redisUsername = username
		return nil
	}
}

// WithRedisPassword set the redis password.
func WithRedisPassword(password string) Option {
	return func(c *WorkerConfig) error {
		if password == "" {
			return errors.New("redis password cannot be empty")
		}
		c.redisPassword = password
		return nil
	}
}

// WithPaymentClient sets the url for the payment service.
func WithPaymentClient(url string) Option {
	return func(c *WorkerConfig) error {
		if url == "" {
			return errors.New("payment url cannot be empty")
		}
		c.paymentURL = url
		return nil
	}
}

// WithConfigStream sets the name of the config stream for the Worker. Defaults submit-config stream.
func WithConfigStream(stream string) Option {
	return func(c *WorkerConfig) error {
		if stream == "" {
			return errors.New("config stream cannot be empty")
		}
		c.configStream = stream
		return nil
	}
}

// CreateWorker is a factory method to create a new instance of submit.Worker.
func CreateWorker(config *WorkerConfig) (*Worker, error) {
	addr := config.redisAddress + ":6379"
	rc := redis.NewClient(addr, config.redisUsername, config.redisPassword)

	csc := payment.NewConfigClient(rc, config.configStream)

	pc, err := payment.NewClient(config.paymentURL)
	if err != nil {
		return nil, fmt.Errorf("error creating payment client: %w", err)
	}

	fy := NewFactory(rc, pc)

	w := NewWorker(csc, fy)

	return w, nil
}

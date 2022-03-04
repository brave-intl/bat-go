package payments

import (
	"context"
	"fmt"

	"github.com/awslabs/amazon-qldb-driver-go/v2/qldbdriver"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/custodian"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/logging"
)

// Service - struct definition of payments service
type Service struct {
	// concurrent safe
	datastore              *qldbdriver.QLDBDriver
	processTransaction     chan Transaction
	stopProcessTransaction func()
	custodians             map[string]custodian.Custodian
}

// LookupVerifier - implement keystore for httpsignature
func (s *Service) LookupVerifier(ctx context.Context, keyID string) (context.Context, *httpsignature.Verifier, error) {
	// TODO: implement
	return ctx, nil, nil
}

// NewService creates a service using the passed datastore and clients configured from the environment
func NewService(ctx context.Context) (context.Context, *Service, error) {
	var logger = logging.Logger(ctx, "payments.NewService")

	driver, err := newQLDBDatastore(ctx)

	if err != nil {
		logger.Fatal().Err(err).Msg("failed to setup qldb")
	}

	// custodian transaction processing channel and stop signal
	// buffer up to 25000 transactions for processing at a time
	processTransaction := make(chan Transaction, 25000)
	ctx, stopProcessTransaction := context.WithCancel(ctx)

	// setup our custodian integrations
	upholdCustodian, err := custodian.New(ctx, custodian.Config{Provider: custodian.Uphold})
	if err != nil {
		logger.Error().Err(err).Msg("failed to create uphold custodian")
		defer stopProcessTransaction()
		return ctx, nil, fmt.Errorf("failed to create uphold custodian: %w", err)
	}
	geminiCustodian, err := custodian.New(ctx, custodian.Config{Provider: custodian.Gemini})
	if err != nil {
		logger.Error().Err(err).Msg("failed to create gemini custodian")
		defer stopProcessTransaction()
		return ctx, nil, fmt.Errorf("failed to create gemini custodian: %w", err)
	}
	bitflyerCustodian, err := custodian.New(ctx, custodian.Config{Provider: custodian.Bitflyer})
	if err != nil {
		logger.Error().Err(err).Msg("failed to create bitflyer custodian")
		defer stopProcessTransaction()
		return ctx, nil, fmt.Errorf("failed to create bitflyer custodian: %w", err)
	}

	service := &Service{
		// initialize qldb datastore
		datastore:              driver,
		processTransaction:     processTransaction,
		stopProcessTransaction: stopProcessTransaction,
		custodians: map[string]custodian.Custodian{
			custodian.Uphold:   upholdCustodian,
			custodian.Gemini:   geminiCustodian,
			custodian.Bitflyer: bitflyerCustodian,
		},
	}

	// startup our transaction processing job
	go func() {
		if err := service.ProcessTransactions(ctx); err != nil {
			logger.Fatal().Err(err).Msg("failed to setup transaction processing job")
		}
	}()

	return ctx, service, nil
}

// ProcessTransactions - read transactions off a channel and process them with custodian
func (s *Service) ProcessTransactions(ctx context.Context) error {
	var logger = logging.Logger(ctx, "payments.ProcessTransactions")

	for {
		select {
		case <-ctx.Done():
			logger.Warn().Msg("context cancelled, no longer processing transactions")
			return nil
		case transaction := <-s.processTransaction:
			logger.Debug().Str("transaction", fmt.Sprintf("%+v", transaction)).Msg("processing a transaction")
			// create a custodian transaction from this transaction:
			custodianTransaction, err := custodian.NewTransaction(
				ctx, transaction.IdempotencyKey, transaction.To, transaction.From, altcurrency.BAT, transaction.Amount,
			)

			if err != nil {
				logger.Error().Err(err).Str("transaction", fmt.Sprintf("%+v", transaction)).Msg("could not create custodian transaction")
				continue
			}

			if c, ok := s.custodians[transaction.Custodian]; ok {
				// TODO: store the full response from submit transaction
				err = c.SubmitTransactions(ctx, custodianTransaction)
				if err != nil {
					logger.Error().Err(err).Str("transaction", fmt.Sprintf("%+v", transaction)).Msg("failed to submit transaction")
					continue
				}
			} else {
				logger.Error().Err(err).Str("transaction", fmt.Sprintf("%+v", transaction)).Msg("invalid custodian")
				continue
			}
		}
	}
}

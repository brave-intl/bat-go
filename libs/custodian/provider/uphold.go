package provider

import (
	"context"

	"github.com/brave-intl/bat-go/libs/logging"
)

// upholdCustodian - implementation of the uphold custodian
type upholdCustodian struct{}

// newUpholdCustodian - create a new uphold custodian with configuration
func newUpholdCustodian(ctx context.Context, conf Config) (*upholdCustodian, error) {
	logger := logging.Logger(ctx, "uphold.newUpholdCustodian")
	logger.Error().Msg("not yet implemented")
	return &upholdCustodian{}, nil
}

// SubmitTransactions - implement Custodian interface
func (uc *upholdCustodian) SubmitTransactions(ctx context.Context, txs ...Transaction) error {
	logger := logging.Logger(ctx, "uphold.SubmitTransactions")
	logger.Error().Msg("not yet implemented")
	return nil
}

// GetTransactionsStatus - implement Custodian interface
func (uc *upholdCustodian) GetTransactionsStatus(ctx context.Context, tx ...Transaction) (map[string]TransactionStatus, error) {
	logger := logging.Logger(ctx, "uphold.GetTransactionsStatus")
	logger.Error().Msg("not yet implemented")
	return nil, nil
}

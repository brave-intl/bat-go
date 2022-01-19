package custodian

import (
	"context"

	errorutils "github.com/brave-intl/bat-go/utils/errors"
)

// upholdCustodian - implementation of the uphold custodian
type upholdCustodian struct{}

// newUpholdCustodian - create a new uphold custodian with configuration
func newUpholdCustodian(ctx context.Context, conf CustodianConfig) (*upholdCustodian, error) {
	return &upholdCustodian{}, errorutils.ErrNotImplemented
}

// SubmitTransactions - implement Custodian interface
func (uc *upholdCustodian) SubmitTransactions(ctx context.Context, txs ...Transaction) error {
	return errorutils.ErrNotImplemented
}

// GetTransactionsStatus - implement Custodian interface
func (uc *upholdCustodian) GetTransactionsStatus(ctx context.Context, tx ...Transaction) (map[string]TransactionStatus, error) {
	return nil, errorutils.ErrNotImplemented
}

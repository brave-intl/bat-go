package custodian

import (
	"context"

	errorutils "github.com/brave-intl/bat-go/utils/errors"
)

// bitflyerCustodian - implementation of the bitflyer custodian
type bitflyerCustodian struct{}

// newBitflyerCustodian - create a new bitflyer custodian with configuration
func newBitflyerCustodian(ctx context.Context, conf CustodianConfig) (*bitflyerCustodian, error) {
	return &bitflyerCustodian{}, errorutils.ErrNotImplemented
}

// SubmitTransactions - implement Custodian interface
func (uc *bitflyerCustodian) SubmitTransactions(ctx context.Context, txs ...Transaction) error {
	return errorutils.ErrNotImplemented
}

// GetTransactionStatus - implement Custodian interface
func (uc *bitflyerCustodian) GetTransactionStatus(ctx context.Context, tx Transaction) (TransactionStatus, error) {
	return nil, errorutils.ErrNotImplemented
}

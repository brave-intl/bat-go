package custodian

import (
	"context"

	errorutils "github.com/brave-intl/bat-go/utils/errors"
)

// geminiCustodian - implementation of the gemini custodian
type geminiCustodian struct{}

// newGeminiCustodian - create a new gemini custodian with configuration
func newGeminiCustodian(ctx context.Context, conf CustodianConfig) (*geminiCustodian, error) {
	return &geminiCustodian{}, errorutils.ErrNotImplemented
}

// SubmitTransactions - implement Custodian interface
func (uc *geminiCustodian) SubmitTransactions(ctx context.Context, txs ...Transaction) error {
	return errorutils.ErrNotImplemented
}

// GetTransactionStatus - implement Custodian interface
func (uc *geminiCustodian) GetTransactionStatus(ctx context.Context, tx Transaction) (TransactionStatus, error) {
	return nil, errorutils.ErrNotImplemented
}

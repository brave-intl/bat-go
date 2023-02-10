package custodian

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/libs/clients/gemini"
	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	loggingutils "github.com/brave-intl/bat-go/libs/logging"
)

// geminiCustodian - implementation of the gemini custodian
type geminiCustodian struct {
	client gemini.Client
}

// newGeminiCustodian - create a new gemini custodian with configuration
func newGeminiCustodian(ctx context.Context, conf Config) (*geminiCustodian, error) {
	logger := loggingutils.Logger(ctx, "custodian.newGeminiCustodian").With().Str("conf", conf.String()).Logger()

	// import config to context if not already set, and create bitflyer client
	geminiClient, err := gemini.NewWithContext(appctx.MapToContext(ctx, conf.Config))
	if err != nil {
		msg := "failed to create client"
		return nil, loggingutils.LogAndError(&logger, msg, fmt.Errorf("%s: %w", msg, err))
	}

	return &geminiCustodian{
		client: geminiClient,
	}, nil
}

// SubmitTransactions - implement Custodian interface
func (uc *geminiCustodian) SubmitTransactions(ctx context.Context, txs ...Transaction) error {
	return errorutils.ErrNotImplemented
}

// GetTransactionsStatus - implement Custodian interface
func (uc *geminiCustodian) GetTransactionsStatus(ctx context.Context, txs ...Transaction) (map[string]TransactionStatus, error) {
	return nil, errorutils.ErrNotImplemented
}

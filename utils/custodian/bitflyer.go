package custodian

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	loggingutils "github.com/brave-intl/bat-go/utils/logging"
)

// bitflyerCustodian - implementation of the bitflyer custodian
type bitflyerCustodian struct {
	client bitflyer.Client
}

// newBitflyerCustodian - create a new bitflyer custodian with configuration
func newBitflyerCustodian(ctx context.Context, conf CustodianConfig) (*bitflyerCustodian, error) {
	logger := loggingutils.Logger(ctx, "custodian.newBitflyerCustodian").With().Str("conf", conf.String()).Logger()

	// import config to context if not already set, and create bitflyer client
	bfClient, err := bitflyer.NewWithContext(appctx.MapToContext(ctx, conf.config))
	if err != nil {
		msg := "failed to create client"
		return nil, loggingutils.LogAndError(&logger, msg, fmt.Errorf("%s: %w", msg, err))
	}

	return &bitflyerCustodian{
		client: bfClient,
	}, nil
}

// SubmitTransactions - implement Custodian interface
func (uc *bitflyerCustodian) SubmitTransactions(ctx context.Context, txs ...Transaction) error {
	return errorutils.ErrNotImplemented
}

type bitflyerTransactionStatus struct {
	Status  string
	Message string
}

func (bts *bitflyerTransactionStatus) String() string {
	return fmt.Sprintf("%s", bts.Status)
}

// GetTransactionsStatus - implement Custodian interface
func (uc *bitflyerCustodian) GetTransactionsStatus(ctx context.Context, txs ...Transaction) (map[string]TransactionStatus, error) {
	logger := loggingutils.Logger(ctx, "bitflyerCustodian.GetTransactionsStatus")

	var transferIDs = []string{}
	for _, tx := range txs {
		transferID, err := tx.GetIdempotencyKey(ctx)
		if err != nil {
			msg := "failed to get idempotency key for transaction"
			return nil, loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
		}
		if transferID != nil {
			transferIDs = append(transferIDs, transferID.String())
		}
	}

	resp, err := uc.client.CheckPayoutStatus(ctx, bitflyer.TransferIDsToBulkStatus(transferIDs))
	if err != nil {
		msg := "failed to perform bitflyer.CheckPayoutStatus"
		return nil, loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
	}
	logger.Debug().Str("resp", fmt.Sprintf("%+v", resp)).Msg("response from check payout status")

	var txStatuses = map[string]TransactionStatus{}
	// reconstruct our response
	if resp != nil {
		if resp.Withdrawals != nil {
			for _, v := range resp.Withdrawals {
				txStatuses[v.TransferID] = &bitflyerTransactionStatus{
					Status: v.Status, Message: v.Message,
				}
			}
		}
	}

	logger.Debug().Str("txStatuses", fmt.Sprintf("%+v", txStatuses)).Msg("result of statuses")

	return txStatuses, nil
}

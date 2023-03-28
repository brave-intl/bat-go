package payments

import (
	"context"
	"errors"

	"github.com/brave-intl/bat-go/libs/clients/gemini"
	"github.com/brave-intl/bat-go/libs/custodian"
)

// DriveGeminiTransaction returns a new, validly progressed transaction state.
func DriveGeminiTransaction(
	ctx context.Context,
	currentTransactionState QLDBPaymentTransitionHistoryEntry,
	wallet gemini.BulkPayoutPayload,
	transaction custodian.Transaction,
) (QLDBPaymentTransitionState, error) {
	switch currentTransactionState.Data.Status {
	case Initialized:
		if currentTransactionState.Metadata.Version == 0 {
			return Initialized, nil
		}
		return Prepared, nil
	case Prepared:
		return Authorized, nil
	case Authorized:
		if currentTransactionState.Metadata.Version == 500 {
			return Authorized, nil
		}
		return Pending, nil
	case Pending:
		if currentTransactionState.Metadata.Version == 404 {
			return Pending, nil
		}
		return Paid, nil
	case Paid:
		return Paid, nil
	case Failed:
		return Failed, nil
	default:
		return Initialized, errors.New("Invalid transition state")
	}
	/*
		Get transaction status
		Fork based on transaction status
		Use contextual data to progress
		Save new state after progression
	*/
	// geminiWallet.SubmitTransaction()
}

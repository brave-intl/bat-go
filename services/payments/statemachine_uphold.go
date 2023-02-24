package payments

import (
	"context"
	"errors"

	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
)

// DriveUpholdTransaction returns a new, validly progressed transaction state.
func DriveUpholdTransaction(
	ctx context.Context,
	currentTransactionState QLDBPaymentTransitionHistoryEntry,
	wallet uphold.Wallet,
	transaction custodian.Transaction,
) (QLDBPaymentTransitionState, error) {
	switch currentTransactionState.Data.Status {
	case 0:
		if currentTransactionState.Metadata.Version == 0 {
			return 0, nil
		}
		return 1, nil
	case 1:
		return 2, nil
	case 2:
		if currentTransactionState.Metadata.Version == 500 {
			return 2, nil
		}
		return 3, nil
	case 3:
		if currentTransactionState.Metadata.Version == 404 {
			return 3, nil
		}
		return 4, nil
	case 4:
		return 4, nil
	case 5:
		return 5, nil
	default:
		return 0, errors.New("Invalid transition state")
	}
	/*
		Get transaction status
		Fork based on transaction status
		Use contextual data to progress
		Save new state after progression
	*/
	// upholdWallet.SubmitTransaction()
}

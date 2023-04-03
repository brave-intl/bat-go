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
	currentTransactionState QLDBPaymentTransitionData,
	currentTransactionVersion int,
	wallet uphold.Wallet,
	transaction custodian.Transaction,
) (QLDBPaymentTransitionState, error) {
	switch currentTransactionState.Status {
	case Initialized:
		if currentTransactionVersion == 0 {
			return Initialized, nil
		}
		return Prepared, nil
	case Prepared:
		return Authorized, nil
	case Authorized:
		if currentTransactionVersion == 500 {
			return Authorized, nil
		}
		return Pending, nil
	case Pending:
		if currentTransactionVersion == 404 {
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
	// upholdWallet.SubmitTransaction()
}

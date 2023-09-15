package payments

import (
	"context"
	"fmt"
)

// BitflyerMachine is an implementation of TxStateMachine for Bitflyer's use-case.
// Including the baseStateMachine provides a default implementation of TxStateMachine,
type BitflyerMachine struct {
	baseStateMachine
}

// Pay implements TxStateMachine for the Bitflyer machine.
func (bm *BitflyerMachine) Pay(ctx context.Context) (*AuthenticatedPaymentState, error) {
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	var (
		entry *AuthenticatedPaymentState
		err   error
	)
	if bm.transaction.Status == Pending {
		return bm.writeNextState(ctx, Paid)
	} else {
		entry, err = bm.writeNextState(ctx, Pending)
		if err != nil {
			return nil, fmt.Errorf("failed to write next state: %w", err)
		}
		entry, err = Drive(ctx, bm)
		if err != nil {
			return nil, fmt.Errorf("failed to drive transaction from pending to paid: %w", err)
		}
	}
	return entry, nil
}

// Fail implements TxStateMachine for the Bitflyer machine.
func (bm *BitflyerMachine) Fail(ctx context.Context) (*AuthenticatedPaymentState, error) {
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	return bm.writeNextState(ctx, Failed)
}

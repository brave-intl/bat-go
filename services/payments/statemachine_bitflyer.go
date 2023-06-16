package payments

import (
	"context"
	"fmt"
)

// BitflyerMachine is an implementation of TxStateMachine for Bitflyer's use-case.
type BitflyerMachine struct {
	baseStateMachine
}

// Prepare implements TxStateMachine for the Bitflyer machine. Bitflyer requires no special
// preparation, so all we do here is progress the state to Prepared.
func (bm *BitflyerMachine) Prepare(ctx context.Context) (*Transaction, error) {
	return bm.writeNextState(ctx, Prepared)
}

// Authorize implements TxStateMachine for the Bitflyer machine.
func (bm *BitflyerMachine) Authorize(ctx context.Context) (*Transaction, error) {
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	return bm.writeNextState(ctx, Authorized)
}

// Pay implements TxStateMachine for the Bitflyer machine.
func (bm *BitflyerMachine) Pay(ctx context.Context) (*Transaction, error) {
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	var (
		entry *Transaction
		err   error
	)
	if bm.transaction.State == Pending {
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
func (bm *BitflyerMachine) Fail(ctx context.Context) (*Transaction, error) {
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	return bm.writeNextState(ctx, Failed)
}

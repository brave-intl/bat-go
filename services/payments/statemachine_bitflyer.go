package payments

import (
	"context"
	"fmt"
)

// BitflyerMachine is an implementation of TxStateMachine for Bitflyer's use-case.
type BitflyerMachine struct {
	baseStateMachine
}

// Prepare implements TxStateMachine for the Bitflyer machine. It will attempt to initialize a record in QLDB
// returning the state of the record in QLDB. If the record already exists, in a state other than Prepared, an
// error is returned.
func (bm *BitflyerMachine) Prepare(ctx context.Context) (*Transaction, error) {
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	entry, err := bm.writeNextState(ctx, Prepared)
	if err != nil {
		return nil, fmt.Errorf("failed to write next state: %w", err)
	}
	return entry, nil
}

// Authorize implements TxStateMachine for the Bitflyer machine.
func (bm *BitflyerMachine) Authorize(ctx context.Context) (*Transaction, error) {
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	entry, err := bm.writeNextState(ctx, Authorized)
	if err != nil {
		return nil, fmt.Errorf("failed to write next state: %w", err)
	}
	return entry, nil
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
		entry, err = bm.writeNextState(ctx, Paid)
		if err != nil {
			return nil, fmt.Errorf("failed to write next state: %w", err)
		}
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
	entry, err := bm.writeNextState(ctx, Failed)
	if err != nil {
		return nil, fmt.Errorf("failed to write next state: %w", err)
	}
	return entry, nil
}

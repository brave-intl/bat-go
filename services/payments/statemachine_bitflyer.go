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
	nextState := Prepared
	if !bm.transaction.nextStateValid(nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", bm.transaction.State, nextState, bm.transaction.ID)
	}
	bm.transaction.State = nextState
	entry, err := bm.wrappedWrite(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	bm.transaction = entry
	return entry, nil
}

// Authorize implements TxStateMachine for the Bitflyer machine.
func (bm *BitflyerMachine) Authorize(ctx context.Context) (*Transaction, error) {
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	nextState := Authorized
	if !bm.transaction.nextStateValid(nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", bm.transaction.State, nextState, bm.transaction.ID)
	}
	bm.transaction.State = nextState
	entry, err := bm.wrappedWrite(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	bm.transaction = entry
	return entry, nil
}

// Pay implements TxStateMachine for the Bitflyer machine.
func (bm *BitflyerMachine) Pay(ctx context.Context) (*Transaction, error) {
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	var (
		nextState TransactionState
		entry     *Transaction
		err       error
	)
	if bm.transaction.State == Pending {
		nextState = Paid
		if !bm.transaction.nextStateValid(nextState) {
			return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", bm.transaction.State, nextState, bm.transaction.ID)
		}
		bm.transaction.State = nextState
		entry, err = bm.wrappedWrite(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to write transaction: %w", err)
		}
	} else {
		nextState = Pending
		if !bm.transaction.nextStateValid(nextState) {
			return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", bm.transaction.State, nextState, bm.transaction.ID)
		}
		bm.transaction.State = nextState
		entry, err = bm.wrappedWrite(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to write transaction: %w", err)
		}
		entry, err = Drive(ctx, bm)
		if err != nil {
			return nil, fmt.Errorf("failed to drive transaction from pending to paid: %w", err)
		}
	}
	bm.transaction = entry
	return entry, nil
}

// Fail implements TxStateMachine for the Bitflyer machine.
func (bm *BitflyerMachine) Fail(ctx context.Context) (*Transaction, error) {
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	nextState := Failed
	if !bm.transaction.nextStateValid(nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", bm.transaction.State, nextState, bm.transaction.ID)
	}
	bm.transaction.State = nextState
	entry, err := bm.wrappedWrite(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	bm.transaction = entry
	return entry, nil
}

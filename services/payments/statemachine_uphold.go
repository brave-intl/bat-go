package payments

import (
	"context"
	"fmt"
)

// UpholdMachine is an implementation of TxStateMachine for uphold's use-case.
type UpholdMachine struct {
	baseStateMachine
}

// Prepare implements TxStateMachine for uphold machine.
func (um *UpholdMachine) Prepare(ctx context.Context) (*Transaction, error) {
	nextState := Prepared
	if !um.transaction.nextStateValid(nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", um.transaction.State, nextState, um.transaction.ID)
	}
	um.transaction.State = nextState
	entry, err := um.wrappedWrite(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	um.transaction = entry
	return entry, nil
}

// Authorize implements TxStateMachine for uphold machine.
func (um *UpholdMachine) Authorize(ctx context.Context) (*Transaction, error) {
	nextState := Authorized
	if !um.transaction.nextStateValid(nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", um.transaction.State, nextState, um.transaction.ID)
	}
	um.transaction.State = nextState
	entry, err := um.wrappedWrite(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	um.transaction = entry
	return entry, nil
}

// Pay implements TxStateMachine for uphold machine.
func (um *UpholdMachine) Pay(ctx context.Context) (*Transaction, error) {
	var nextState TransactionState
	if um.transaction.State == Pending {
		nextState = Paid
		if !um.transaction.nextStateValid(nextState) {
			return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", um.transaction.State, nextState, um.transaction.ID)
		}
		um.transaction.State = nextState
	} else {
		nextState = Pending
		if !um.transaction.nextStateValid(nextState) {
			return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", um.transaction.State, nextState, um.transaction.ID)
		}
		um.transaction.State = nextState
	}
	entry, err := um.wrappedWrite(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	um.transaction = entry
	return entry, nil
}

// Fail implements TxStateMachine for uphold machine.
func (um *UpholdMachine) Fail(ctx context.Context) (*Transaction, error) {
	nextState := Failed
	if !um.transaction.nextStateValid(nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", um.transaction.State, nextState, um.transaction.ID)
	}
	um.transaction.State = nextState
	entry, err := um.wrappedWrite(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	um.transaction = entry
	return entry, nil
}

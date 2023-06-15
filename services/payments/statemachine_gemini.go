package payments

import (
	"context"
	"fmt"
)

// GeminiMachine is an implementation of TxStateMachine for Gemini's use-case.
type GeminiMachine struct {
	baseStateMachine
}

// Prepare implements TxStateMachine for the Gemini machine.
func (gm *GeminiMachine) Prepare(ctx context.Context) (*Transaction, error) {
	nextState := Prepared
	if !gm.transaction.nextStateValid(nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", gm.transaction.State, nextState, gm.transaction.ID)
	}
	gm.transaction.State = nextState
	entry, err := gm.wrappedWrite(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	gm.transaction = entry
	return entry, nil
}

// Authorize implements TxStateMachine for the Gemini machine.
func (gm *GeminiMachine) Authorize(ctx context.Context) (*Transaction, error) {
	nextState := Authorized
	if !gm.transaction.nextStateValid(nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", gm.transaction.State, nextState, gm.transaction.ID)
	}
	gm.transaction.State = nextState
	entry, err := gm.wrappedWrite(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	gm.transaction = entry
	return entry, nil
}

// Pay implements TxStateMachine for the Gemini machine.
func (gm *GeminiMachine) Pay(ctx context.Context) (*Transaction, error) {
	var (
		nextState TransactionState
		entry     *Transaction
		err       error
	)
	if gm.transaction.State == Pending {
		nextState = Paid
		if !gm.transaction.nextStateValid(nextState) {
			return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", gm.transaction.State, nextState, gm.transaction.ID)
		}
		gm.transaction.State = nextState
		entry, err = gm.wrappedWrite(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to write transaction: %w", err)
		}
	} else {
		nextState = Pending
		if !gm.transaction.nextStateValid(nextState) {
			return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", gm.transaction.State, nextState, gm.transaction.ID)
		}
		gm.transaction.State = nextState
		entry, err = gm.wrappedWrite(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to write transaction: %w", err)
		}
		entry, err = Drive(ctx, gm)
		if err != nil {
			return nil, fmt.Errorf("failed to drive transaction from pending to paid: %w", err)
		}
	}
	gm.transaction = entry
	return entry, nil
}

// Fail implements TxStateMachine for the Gemini machine.
func (gm *GeminiMachine) Fail(ctx context.Context) (*Transaction, error) {
	nextState := Failed
	if !gm.transaction.nextStateValid(nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", gm.transaction.State, nextState, gm.transaction.ID)
	}
	gm.transaction.State = nextState
	entry, err := gm.wrappedWrite(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	gm.transaction = entry
	return entry, nil
}

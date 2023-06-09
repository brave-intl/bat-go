package payments

import (
	"context"
	"fmt"
)

// GeminiMachine is an implementation of TxStateMachine for Gemini's use-case.
type GeminiMachine struct {
	baseStateMachine
}

// NewGeminiMachine returns an GeminiMachine with values specified.
func NewGeminiMachine(transaction *Transaction, service *Service) *GeminiMachine {
	machine := GeminiMachine{}
	machine.setService(service)
	machine.setTransaction(transaction)
	return &machine
}

// Prepare implements TxStateMachine for the Gemini machine.
func (gm *GeminiMachine) Prepare(ctx context.Context) (*Transaction, error) {
	nextState := Prepared
	if !gm.transaction.nextStateValid(nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", gm.transaction.State, nextState, gm.transaction.ID)
	}
	gm.transaction.State = nextState
	entry, err := gm.service.WriteTransaction(ctx, gm.transaction)
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
	entry, err := gm.service.WriteTransaction(ctx, gm.transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	gm.transaction = entry
	return entry, nil
}

// Pay implements TxStateMachine for the Gemini machine.
func (gm *GeminiMachine) Pay(ctx context.Context) (*Transaction, error) {
	var nextState TransactionState
	if gm.transaction.State == Pending {
		nextState = Paid
		if !gm.transaction.nextStateValid(nextState) {
			return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", gm.transaction.State, nextState, gm.transaction.ID)
		}
		gm.transaction.State = nextState
	} else {
		nextState = Pending
		if !gm.transaction.nextStateValid(nextState) {
			return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", gm.transaction.State, nextState, gm.transaction.ID)
		}
		gm.transaction.State = nextState
	}
	entry, err := gm.service.WriteTransaction(ctx, gm.transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
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
	entry, err := gm.service.WriteTransaction(ctx, gm.transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	gm.transaction = entry
	return entry, nil
}

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
	entry, err := gm.writeNextState(ctx, Prepared)
	if err != nil {
		return nil, fmt.Errorf("failed to write next state: %w", err)
	}
	return entry, nil
}

// Authorize implements TxStateMachine for the Gemini machine.
func (gm *GeminiMachine) Authorize(ctx context.Context) (*Transaction, error) {
	entry, err := gm.writeNextState(ctx, Authorized)
	if err != nil {
		return nil, fmt.Errorf("failed to write next state: %w", err)
	}
	return entry, nil
}

// Pay implements TxStateMachine for the Gemini machine.
func (gm *GeminiMachine) Pay(ctx context.Context) (*Transaction, error) {
	var (
		entry *Transaction
		err   error
	)
	if gm.transaction.State == Pending {
		entry, err = gm.writeNextState(ctx, Paid)
		if err != nil {
			return nil, fmt.Errorf("failed to write next state: %w", err)
		}
	} else {
		entry, err = gm.writeNextState(ctx, Pending)
		if err != nil {
			return nil, fmt.Errorf("failed to write next state: %w", err)
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
	entry, err := gm.writeNextState(ctx, Failed)
	if err != nil {
		return nil, fmt.Errorf("failed to write next state: %w", err)
	}
	return entry, nil
}

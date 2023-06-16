package payments

import (
	"context"
	"fmt"
)

// GeminiMachine is an implementation of TxStateMachine for Gemini's use-case.
type GeminiMachine struct {
	baseStateMachine
}

// Pay implements TxStateMachine for the Gemini machine.
func (gm *GeminiMachine) Pay(ctx context.Context) (*Transaction, error) {
	var (
		entry *Transaction
		err   error
	)
	if gm.transaction.State == Pending {
		return gm.writeNextState(ctx, Paid)
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
	return entry, nil
}

// Fail implements TxStateMachine for the Gemini machine.
func (gm *GeminiMachine) Fail(ctx context.Context) (*Transaction, error) {
	return gm.writeNextState(ctx, Failed)
}

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
	return um.writeNextState(ctx, Prepared)
}

// Authorize implements TxStateMachine for uphold machine.
func (um *UpholdMachine) Authorize(ctx context.Context) (*Transaction, error) {
	return um.writeNextState(ctx, Authorized)
}

// Pay implements TxStateMachine for uphold machine.
func (um *UpholdMachine) Pay(ctx context.Context) (*Transaction, error) {
	var (
		entry *Transaction
		err   error
	)
	if um.transaction.State == Pending {
		return um.writeNextState(ctx, Paid)
	} else {
		entry, err = um.writeNextState(ctx, Pending)
		if err != nil {
			return nil, fmt.Errorf("failed to write next state: %w", err)
		}
		entry, err = Drive(ctx, um)
		if err != nil {
			return nil, fmt.Errorf("failed to drive transaction from pending to paid: %w", err)
		}
	}
	return entry, nil
}

// Fail implements TxStateMachine for uphold machine.
func (um *UpholdMachine) Fail(ctx context.Context) (*Transaction, error) {
	return um.writeNextState(ctx, Failed)
}

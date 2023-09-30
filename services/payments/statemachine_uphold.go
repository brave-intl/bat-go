package payments

import (
	"context"
	"fmt"

	. "github.com/brave-intl/bat-go/libs/payments"
)

// UpholdMachine is an implementation of TxStateMachine for uphold's use-case.
// Including the baseStateMachine provides a default implementation of TxStateMachine,
type UpholdMachine struct {
	baseStateMachine
}

// Pay implements TxStateMachine for uphold machine.
func (um *UpholdMachine) Pay(ctx context.Context) (*AuthenticatedPaymentState, error) {
	var (
		entry *AuthenticatedPaymentState
		err   error
	)
	if um.transaction.Status == Pending {
		return um.SetNextState(ctx, Paid)
	} else {
		entry, err = um.SetNextState(ctx, Pending)
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
func (um *UpholdMachine) Fail(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return um.SetNextState(ctx, Failed)
}

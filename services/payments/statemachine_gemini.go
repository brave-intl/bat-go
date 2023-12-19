package payments

import (
	"context"
	"fmt"

	paymentLib "github.com/brave-intl/bat-go/libs/payments"
)

// GeminiMachine is an implementation of TxStateMachine for Gemini's use-case.
// Including the baseStateMachine provides a default implementation of TxStateMachine,
type GeminiMachine struct {
	baseStateMachine
}

// Pay implements TxStateMachine for the Gemini machine.
func (gm *GeminiMachine) Pay(ctx context.Context) (*paymentLib.AuthenticatedPaymentState, error) {
	var (
		entry *paymentLib.AuthenticatedPaymentState
		err   error
	)
	if gm.transaction.Status == paymentLib.Pending {
		return gm.SetNextState(ctx, paymentLib.Paid)
	} else {
		entry, err = gm.SetNextState(ctx, paymentLib.Pending)
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
func (gm *GeminiMachine) Fail(ctx context.Context) (*paymentLib.AuthenticatedPaymentState, error) {
	return gm.SetNextState(ctx, paymentLib.Failed)
}

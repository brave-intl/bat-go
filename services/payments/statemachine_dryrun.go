package payments

import (
	"context"
	"errors"

	. "github.com/brave-intl/bat-go/libs/payments"
)

// HappyPathMachine is an implementation of TxStateMachine for a happy path dry-run
type HappyPathMachine struct {
	baseStateMachine
}

// Pay implements TxStateMachine for the HappyPathMachine.
func (s *HappyPathMachine) Pay(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return s.SetNextState(ctx, Paid)
}

// Fail implements TxStateMachine for the HappyPathMachine.
func (s *HappyPathMachine) Fail(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return s.SetNextState(ctx, Failed)
}

// PrepareFailsMachine is an implementation of TxStateMachine for a dry-run with a failing submit
type PrepareFailsMachine struct {
	HappyPathMachine
}

// Prepare implements TxStateMachine for the baseStateMachine.
func (s *PrepareFailsMachine) Prepare(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return s.getTransaction(), errors.New("dry-run authorize fails")
}

// AuthorizeFailsMachine is an implementation of TxStateMachine for a dry-run with a failing submit
type AuthorizeFailsMachine struct {
	HappyPathMachine
}

// Authorize implements TxStateMachine for the baseStateMachine.
func (s *AuthorizeFailsMachine) Authorize(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return s.getTransaction(), errors.New("dry-run authorize fails")
}

// PayFailsMachine is an implementation of TxStateMachine for a dry-run with a failing submit
type PayFailsMachine struct {
	HappyPathMachine
}

// Authorize implements TxStateMachine for the baseStateMachine.
func (s *PayFailsMachine) Authorize(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return s.SetNextState(ctx, Authorized)
}

// Pay implements TxStateMachine for the baseStateMachine.
func (s *PayFailsMachine) Pay(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return s.getTransaction(), errors.New("dry-run pay fails")
}

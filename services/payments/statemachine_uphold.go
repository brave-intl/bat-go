package payments

import (
	"context"
	"fmt"
	"github.com/google/uuid"
)

// UpholdMachine is an implementation of TxStateMachine for uphold's use-case
type UpholdMachine struct {
	transaction *Transaction
	service     *Service
}

// NewUpholdMachine returns an UpholdMachine with values specified
func NewUpholdMachine(transaction *Transaction, service *Service) *UpholdMachine {
	machine := UpholdMachine{}
	machine.setService(service)
	machine.setTransaction(transaction)
	return &machine
}

// setTransaction assigns the transaction field in the UpholdMachine to the specified Transaction
func (um *UpholdMachine) setTransaction(transaction *Transaction) {
	um.transaction = transaction
}

// setConnection assigns the connection field in the UpholdMachine to the specified wrappedQldbDriverAPI
func (um *UpholdMachine) setService(service *Service) {
	um.service = service
}

// GetState returns the state of the machine's associated transaction
func (um *UpholdMachine) GetState() TransactionState {
	return um.transaction.State
}

// GetService returns the service associated with the machine
func (um *UpholdMachine) GetService() *Service {
	return um.service
}

// GetTransactionID returns the ID that is on the associated transaction
func (um *UpholdMachine) GetTransactionID() *uuid.UUID {
	return um.transaction.ID
}

// GenerateTransactionID returns an ID generated from the values of the transaction
func (um *UpholdMachine) GenerateTransactionID(ctx context.Context) (*uuid.UUID, error) {
	return um.transaction.GenerateIdempotencyKey(ctx)
}

// Prepare implements TxStateMachine for uphold machine
func (um *UpholdMachine) Prepare(ctx context.Context) (*Transaction, error) {
	nextState := Prepared
	if !nextStateValid(um.transaction, nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", um.transaction.State, nextState, um.transaction.ID)
	}
	um.transaction.State = nextState
	entry, err := um.service.WriteTransaction(ctx, um.transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	um.transaction = entry
	return entry, nil
}

// Authorize implements TxStateMachine for uphold machine
func (um *UpholdMachine) Authorize(ctx context.Context) (*Transaction, error) {
	nextState := Authorized
	if !nextStateValid(um.transaction, nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", um.transaction.State, nextState, um.transaction.ID)
	}
	um.transaction.State = nextState
	entry, err := um.service.WriteTransaction(ctx, um.transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	um.transaction = entry
	return entry, nil
}

// Pay implements TxStateMachine for uphold machine
func (um *UpholdMachine) Pay(ctx context.Context) (*Transaction, error) {
	var nextState TransactionState
	if um.transaction.State == Pending {
		nextState = Paid
		if !nextStateValid(um.transaction, nextState) {
			return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", um.transaction.State, nextState, um.transaction.ID)
		}
		um.transaction.State = nextState
	} else {
		nextState = Pending
		if !nextStateValid(um.transaction, nextState) {
			return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", um.transaction.State, nextState, um.transaction.ID)
		}
		um.transaction.State = nextState
	}
	entry, err := um.service.WriteTransaction(ctx, um.transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	um.transaction = entry
	return entry, nil
}

// Fail implements TxStateMachine for uphold machine
func (um *UpholdMachine) Fail(ctx context.Context) (*Transaction, error) {
	nextState := Failed
	if !nextStateValid(um.transaction, nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", um.transaction.State, nextState, um.transaction.ID)
	}
	um.transaction.State = nextState
	entry, err := um.service.WriteTransaction(ctx, um.transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	um.transaction = entry
	return entry, nil
}

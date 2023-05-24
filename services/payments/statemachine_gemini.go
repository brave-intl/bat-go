package payments

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// GeminiMachine is an implementation of TxStateMachine for Gemini's use-case
type GeminiMachine struct {
	transaction *Transaction
	service     *Service
}

// NewGeminiMachine returns an GeminiMachine with values specified
func NewGeminiMachine(transaction *Transaction, service *Service) *GeminiMachine {
	machine := GeminiMachine{}
	machine.setService(service)
	machine.setTransaction(transaction)
	return &machine
}

// setTransaction assigns the transaction field in the GeminiMachine to the specified Transaction
func (gm *GeminiMachine) setTransaction(transaction *Transaction) {
	gm.transaction = transaction
}

// setConnection assigns the connection field in the GeminiMachine to the specified wrappedQldbDriverAPI
func (gm *GeminiMachine) setService(service *Service) {
	gm.service = service
}

// GetState returns the state of the machine's associated transaction
func (gm *GeminiMachine) GetState() TransactionState {
	return gm.transaction.State
}

// GetService returns the service associated with the machine
func (gm *GeminiMachine) GetService() *Service {
	return gm.service
}

// GetTransactionID returns the ID that is on the associated transaction
func (gm *GeminiMachine) GetTransactionID() *uuid.UUID {
	return gm.transaction.ID
}

// GenerateTransactionID returns an ID generated from the values of the transaction
func (gm *GeminiMachine) GenerateTransactionID(namespace uuid.UUID) (*uuid.UUID, error) {
	return gm.transaction.GenerateIdempotencyKey(namespace)
}

// Prepare implements TxStateMachine for the Gemini machine
func (gm *GeminiMachine) Prepare(ctx context.Context) (*Transaction, error) {
	nextState := Prepared
	if !nextStateValid(gm.transaction, nextState) {
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

// Authorize implements TxStateMachine for the Gemini machine
func (gm *GeminiMachine) Authorize(ctx context.Context) (*Transaction, error) {
	nextState := Authorized
	if !nextStateValid(gm.transaction, nextState) {
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

// Pay implements TxStateMachine for the Gemini machine
func (gm *GeminiMachine) Pay(ctx context.Context) (*Transaction, error) {
	var nextState TransactionState
	if gm.transaction.State == Pending {
		nextState = Paid
		if !nextStateValid(gm.transaction, nextState) {
			return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", gm.transaction.State, nextState, gm.transaction.ID)
		}
		gm.transaction.State = nextState
	} else {
		nextState = Pending
		if !nextStateValid(gm.transaction, nextState) {
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

// Fail implements TxStateMachine for the Gemini machine
func (gm *GeminiMachine) Fail(ctx context.Context) (*Transaction, error) {
	nextState := Failed
	if !nextStateValid(gm.transaction, nextState) {
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

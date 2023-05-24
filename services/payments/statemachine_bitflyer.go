package payments

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// BitflyerMachine is an implementation of TxStateMachine for Bitflyer's use-case
type BitflyerMachine struct {
	transaction *Transaction
	service     *Service
}

// NewBitflyerMachine returns an BitflyerMachine with values specified
func NewBitflyerMachine(transaction *Transaction, service *Service) *BitflyerMachine {
	machine := BitflyerMachine{}
	machine.setService(service)
	machine.setTransaction(transaction)
	return &machine
}

// setTransaction assigns the transaction field in the BitflyerMachine to the specified Transaction
func (bm *BitflyerMachine) setTransaction(transaction *Transaction) {
	bm.transaction = transaction
}

// setConnection assigns the connection field in the BitflyerMachine to the specified wrappedQldbDriverAPI
func (bm *BitflyerMachine) setService(service *Service) {
	bm.service = service
}

// GetState returns the state of the machine's associated transaction
func (bm *BitflyerMachine) GetState() TransactionState {
	return bm.transaction.State
}

// GetService returns the service associated with the machine
func (bm *BitflyerMachine) GetService() *Service {
	return bm.service
}

// GetTransactionID returns the ID that is on the associated transaction
func (bm *BitflyerMachine) GetTransactionID() *uuid.UUID {
	return bm.transaction.ID
}

// GenerateTransactionID returns an ID generated from the values of the transaction
func (bm *BitflyerMachine) GenerateTransactionID(namespace uuid.UUID) (*uuid.UUID, error) {
	return bm.transaction.GenerateIdempotencyKey(namespace)
}

// Prepare implements TxStateMachine for the Bitflyer machine. It will attempt to initialize a record in QLDB
// returning the state of the record in QLDB. If the record already exists, in a state other than Prepared, an
// error is returned.
func (bm *BitflyerMachine) Prepare(ctx context.Context) (*Transaction, error) {
	/*if !shouldDryRun(bm.transaction) {
		// Do bitflyer stuff
	}*/
	nextState := Prepared
	if !nextStateValid(bm.transaction, nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", bm.transaction.State, nextState, bm.transaction.ID)
	}
	bm.transaction.State = nextState
	entry, err := bm.service.WriteTransaction(ctx, bm.transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	bm.transaction = entry
	return entry, nil
}

// Authorize implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Authorize(ctx context.Context) (*Transaction, error) {
	/*if !shouldDryRun(bm.transaction) {
		// Do bitflyer stuff
	}*/
	nextState := Authorized
	if !nextStateValid(bm.transaction, nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", bm.transaction.State, nextState, bm.transaction.ID)
	}
	bm.transaction.State = nextState
	entry, err := bm.service.WriteTransaction(ctx, bm.transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	bm.transaction = entry
	return entry, nil
}

// Pay implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Pay(ctx context.Context) (*Transaction, error) {
	/*if !shouldDryRun(bm.transaction) {
		// Do bitflyer stuff
	}*/
	var nextState TransactionState
	if bm.transaction.State == Pending {
		nextState = Paid
		if !nextStateValid(bm.transaction, nextState) {
			return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", bm.transaction.State, nextState, bm.transaction.ID)
		}
		bm.transaction.State = nextState
	} else {
		nextState = Pending
		if !nextStateValid(bm.transaction, nextState) {
			return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", bm.transaction.State, nextState, bm.transaction.ID)
		}
		bm.transaction.State = nextState
	}
	entry, err := bm.service.WriteTransaction(ctx, bm.transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	bm.transaction = entry
	return entry, nil
}

// Fail implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Fail(ctx context.Context) (*Transaction, error) {
	/*if !shouldDryRun(bm.transaction) {
		// Do bitflyer stuff
	}*/
	nextState := Failed
	if !nextStateValid(bm.transaction, nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", bm.transaction.State, nextState, bm.transaction.ID)
	}
	bm.transaction.State = nextState
	entry, err := bm.service.WriteTransaction(ctx, bm.transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	bm.transaction = entry
	return entry, nil
}

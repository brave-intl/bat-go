package payments

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// BitflyerMachine is an implementation of TxStateMachine for Bitflyer's use-case
type BitflyerMachine struct {
	// client wallet.Bitflyer
	// transactionSet bitflyer.WithdrawToDepositIDBulkPayload
	version             int
	transaction         *Transaction
	connection          wrappedQldbDriverAPI
	kmsSigningKeyClient *kms.Client
}

// setVersion assigns the version field in the BitflyerMachine to the specified int
func (bm *BitflyerMachine) setVersion(version int) {
	bm.version = version
}

// setTransaction assigns the transaction field in the BitflyerMachine to the specified Transaction
func (bm *BitflyerMachine) setTransaction(transaction *Transaction) {
	bm.transaction = transaction
}

// setConnection assigns the connection field in the BitflyerMachine to the specified wrappedQldbDriverAPI
func (bm *BitflyerMachine) setConnection(connection wrappedQldbDriverAPI) {
	bm.connection = connection
}

// Initialized implements TxStateMachine for the Bitflyer machine. It will attempt to Initialize a record in QLDB
// returning the state of the record in QLDB. If the record already exists, in a state other than Initialize, an
// error is returned.
func (bm *BitflyerMachine) Initialized() (TransactionState, error) {
	//if !idempotencyKeyIsValid(bm.transaction) {
	//	return Initialized, errors.New("provided idempotencyKey does not match transaction")
	//}
	ctx := context.Background()
	// Attempt to write
	entry, err := WriteQLDBObject(ctx, bm.connection, nil, bm.kmsSigningKeyClient, bm.transaction)
	if err != nil {
		return Initialized, fmt.Errorf("failed to write transaction: %w", err)
	}
	if entry.State != Initialized {
		return entry.State, fmt.Errorf("QLDB record exists and is in %s state", entry.State.String())
	}
	return entry.State, nil
}

// Prepared implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Prepared() (TransactionState, error) {
	// if failure, do failed branch
	if false {
		return Failed, nil
	}
	return Authorized, nil
}

// Authorized implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Authorized() (TransactionState, error) {
	if bm.version == 500 {
		return Authorized, nil
	}
	return Pending, nil
}

// Pending implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Pending() (TransactionState, error) {
	if bm.version == 404 {
		return Pending, nil
	}
	return Paid, nil
}

// Paid implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Paid() (TransactionState, error) {
	return Paid, nil
}

// Failed implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Failed() (TransactionState, error) {
	return Failed, nil
}

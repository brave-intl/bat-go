package payments

import (
	"context"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
)

// BitflyerMachine is an implementation of TxStateMachine for Bitflyer's use-case
type BitflyerMachine struct {
	// client wallet.Bitflyer
	// transactionSet bitflyer.WithdrawToDepositIDBulkPayload
	version     int
	transaction *Transaction
}

// setVersion assigns the version field in the BitflyerMachine to the specified int
func (bm *BitflyerMachine) setVersion(version int) {
	bm.version = version
}

// setTransaction assigns the transaction field in the BitflyerMachine to the specified Transaction
func (bm *BitflyerMachine) setTransaction(transaction *Transaction) {
	bm.transaction = transaction
}

// Initialized implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Initialized() (TransactionState, error) {
	p, err := qldbdriver.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		result, err := txn.Execute("SELECT firstName, lastName, age FROM People WHERE age = 54")
		if err != nil {
			return nil, err
		}

		// Assume the result is not empty
		hasNext := result.Next(txn)
		if !hasNext && result.Err() != nil {
			return nil, result.Err()
		}

		ionBinary := result.GetCurrentData()

		temp := new(Person)
		err = ion.Unmarshal(ionBinary, temp)
		if err != nil {
			return nil, err
		}

		return *temp, nil
	})
	GetQLDBObject(bm.transaction.DocumentID)
	if bm.version == 0 {
		return Initialized, nil
	}
	return Prepared, nil
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

package payments

// UpholdMachine is an implementation of TxStateMachine for uphold's use-case
type UpholdMachine struct {
	// client uphold.Wallet
	// transaction custodian.Transaction
	version     int
	transaction *Transaction
	connection  wrappedQldbDriverAPI
}

// setVersion assigns the version field in the UpholdMachine to the specified int
func (um *UpholdMachine) setVersion(version int) {
	um.version = version
}

// setTransaction assigns the transaction field in the UpholdMachine to the specified Transaction
func (um *UpholdMachine) setTransaction(transaction *Transaction) {
	um.transaction = transaction
}

// setConnection assigns the connection field in the UpholdMachine to the specified wrappedQldbDriverAPI
func (um *UpholdMachine) setConnection(connection wrappedQldbDriverAPI) {
	um.connection = connection
}

// Initialized implements TxStateMachine for uphold machine
func (um *UpholdMachine) Initialized() (TransactionState, error) {
	if um.version == 0 {
		return Initialized, nil
	}
	return Prepared, nil
}

// Prepared implements TxStateMachine for uphold machine
func (um *UpholdMachine) Prepared() (TransactionState, error) {
	// if failure, do failed branch
	if false {
		return Failed, nil
	}
	return Authorized, nil
}

// Authorized implements TxStateMachine for uphold machine
func (um *UpholdMachine) Authorized() (TransactionState, error) {
	if um.version == 500 {
		return Authorized, nil
	}
	return Pending, nil
}

// Pending implements TxStateMachine for uphold machine
func (um *UpholdMachine) Pending() (TransactionState, error) {
	if um.version == 404 {
		return Pending, nil
	}
	return Paid, nil
}

// Paid implements TxStateMachine for uphold machine
func (um *UpholdMachine) Paid() (TransactionState, error) {
	return Paid, nil
}

// Failed implements TxStateMachine for uphold machine
func (um *UpholdMachine) Failed() (TransactionState, error) {
	return Failed, nil
}

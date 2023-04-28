package payments

// GeminiMachine is an implementation of TxStateMachine for Gemini's use-case
type GeminiMachine struct {
	// client wallet gemini.BulkPayoutPayload
	// transaction custodian.Transaction
	version     int
	transaction *Transaction
	connection  wrappedQldbDriverAPI
}

// setVersion assigns the version field in the GeminiMachine to the specified int
func (gm *GeminiMachine) setVersion(version int) {
	gm.version = version
}

// setTransaction assigns the transaction field in the GeminiMachine to the specified Transaction
func (gm *GeminiMachine) setTransaction(transaction *Transaction) {
	gm.transaction = transaction
}

// setConnection assigns the connection field in the GeminiMachine to the specified wrappedQldbDriverAPI
func (gm *GeminiMachine) setConnection(connection wrappedQldbDriverAPI) {
	gm.connection = connection
}

// Initialized implements TxStateMachine for the Gemini machine
func (gm *GeminiMachine) Initialized() (TransactionState, error) {
	if gm.version == 0 {
		return Initialized, nil
	}
	return Prepared, nil
}

// Prepared implements TxStateMachine for the Gemini machine
func (gm *GeminiMachine) Prepared() (TransactionState, error) {
	// if failure, do failed branch
	if false {
		return Failed, nil
	}
	return Authorized, nil
}

// Authorized implements TxStateMachine for the Gemini machine
func (gm *GeminiMachine) Authorized() (TransactionState, error) {
	if gm.version == 500 {
		return Authorized, nil
	}
	return Pending, nil
}

// Pending implements TxStateMachine for the Gemini machine
func (gm *GeminiMachine) Pending() (TransactionState, error) {
	if gm.version == 404 {
		return Pending, nil
	}
	return Paid, nil
}

// Paid implements TxStateMachine for the Gemini machine
func (gm *GeminiMachine) Paid() (TransactionState, error) {
	return Paid, nil
}

// Failed implements TxStateMachine for the Gemini machine
func (gm *GeminiMachine) Failed() (TransactionState, error) {
	return Failed, nil
}

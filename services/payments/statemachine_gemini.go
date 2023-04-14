package payments

// GeminiMachine is an implementation of TxStateMachine for uphold's use-case
type GeminiMachine struct {
	// client wallet gemini.BulkPayoutPayload
	// transaction custodian.Transaction
	version int
}

// SetVersion assigns the version field in the GeminiMachine to the specified int
func (gm *GeminiMachine) SetVersion(version int) {
	gm.version = version
}

// Initialized implements TxStateMachine for uphold machine
func (gm *GeminiMachine) Initialized() (QLDBPaymentTransitionState, error) {
	if gm.version == 0 {
		return Initialized, nil
	}
	return Prepared, nil
}

// Prepared implements TxStateMachine for uphold machine
func (gm *GeminiMachine) Prepared() (QLDBPaymentTransitionState, error) {
	// if failure, do failed branch
	if false {
		return Failed, nil
	}
	return Authorized, nil
}

// Authorized implements TxStateMachine for uphold machine
func (gm *GeminiMachine) Authorized() (QLDBPaymentTransitionState, error) {
	if gm.version == 500 {
		return Authorized, nil
	}
	return Pending, nil
}

// Pending implements TxStateMachine for uphold machine
func (gm *GeminiMachine) Pending() (QLDBPaymentTransitionState, error) {
	if gm.version == 404 {
		return Pending, nil
	}
	return Paid, nil
}

// Paid implements TxStateMachine for uphold machine
func (gm *GeminiMachine) Paid() (QLDBPaymentTransitionState, error) {
	return Paid, nil
}

// Failed implements TxStateMachine for uphold machine
func (gm *GeminiMachine) Failed() (QLDBPaymentTransitionState, error) {
	return Failed, nil
}

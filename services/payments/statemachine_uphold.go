package payments

// UpholdMachine is an implementation of TxStateMachine for uphold's use-case
type UpholdMachine struct {
	// client uphold.Wallet
	// transaction custodian.Transaction
	version int
}

// SetVersion assigns the version field in the GeminiMachine to the specified int
func (um *UpholdMachine) SetVersion(version int) {
	um.version = version
}

// Initialized implements TxStateMachine for uphold machine
func (um *UpholdMachine) Initialized() (QLDBPaymentTransitionState, error) {
	if um.version == 0 {
		return Initialized, nil
	}
	return Prepared, nil
}

// Prepared implements TxStateMachine for uphold machine
func (um *UpholdMachine) Prepared() (QLDBPaymentTransitionState, error) {
	// if failure, do failed branch
	if false {
		return Failed, nil
	}
	return Authorized, nil
}

// Authorized implements TxStateMachine for uphold machine
func (um *UpholdMachine) Authorized() (QLDBPaymentTransitionState, error) {
	if um.version == 500 {
		return Authorized, nil
	}
	return Pending, nil
}

// Pending implements TxStateMachine for uphold machine
func (um *UpholdMachine) Pending() (QLDBPaymentTransitionState, error) {
	if um.version == 404 {
		return Pending, nil
	}
	return Paid, nil
}

// Paid implements TxStateMachine for uphold machine
func (um *UpholdMachine) Paid() (QLDBPaymentTransitionState, error) {
	return Paid, nil
}

// Failed implements TxStateMachine for uphold machine
func (um *UpholdMachine) Failed() (QLDBPaymentTransitionState, error) {
	return Failed, nil
}

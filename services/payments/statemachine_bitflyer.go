package payments

// BitflyerMachine is an implementation of TxStateMachine for Bitflyer's use-case
type BitflyerMachine struct {
	// client wallet.Bitflyer
	// transactionSet bitflyer.WithdrawToDepositIDBulkPayload
	version int
}

// SetVersion assigns the version field in the BitflyerMachine to the specified int
func (bm *BitflyerMachine) SetVersion(version int) {
	bm.version = version
}

// Initialized implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Initialized() (QLDBPaymentTransitionState, error) {
	if bm.version == 0 {
		return Initialized, nil
	}
	return Prepared, nil
}

// Prepared implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Prepared() (QLDBPaymentTransitionState, error) {
	// if failure, do failed branch
	if false {
		return Failed, nil
	}
	return Authorized, nil
}

// Authorized implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Authorized() (QLDBPaymentTransitionState, error) {
	if bm.version == 500 {
		return Authorized, nil
	}
	return Pending, nil
}

// Pending implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Pending() (QLDBPaymentTransitionState, error) {
	if bm.version == 404 {
		return Pending, nil
	}
	return Paid, nil
}

// Paid implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Paid() (QLDBPaymentTransitionState, error) {
	return Paid, nil
}

// Failed implements TxStateMachine for the Bitflyer machine
func (bm *BitflyerMachine) Failed() (QLDBPaymentTransitionState, error) {
	return Failed, nil
}

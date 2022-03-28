package custodian

import (
	"github.com/brave-intl/bat-go/utils/clients/payment"
)

const (
	// Gemini custodian
	Gemini = "gemini"
	// Uphold custodian
	Uphold = "uphold"
	// Bitflyer custodian
	Bitflyer = "bitflyer"

	// Complete represents the transactions state Complete
	Complete = TransactionState("complete")
	// Pending represents the transactions state Pending
	Pending = TransactionState("pending")
	// Failed represents the transactions state Failed
	Failed = TransactionState("failed")
	// Errored represents the transactions state Errored
	Errored = TransactionState("errored")
)

type (
	// TransactionState implement
	TransactionState string
	// StatusResolver implement
	StatusResolver func(transactionStatus payment.TransactionStatus) (TransactionState, error)
)

// CheckCustodianStatusResponse implement
func CheckCustodianStatusResponse(transactionStatus payment.TransactionStatus) (TransactionState, error) {
	return Complete, nil
}

// CheckCustodianSubmitResponse implement
func CheckCustodianSubmitResponse(transactionStatus payment.TransactionStatus) (TransactionState, error) {
	return Complete, nil
}

package custodian

import (
	"github.com/brave-intl/bat-go/utils/clients/payment"
)

const (
	// custodians
	Gemini   = "gemini"
	Uphold   = "uphold"
	Bitflyer = "bitflyer"

	// states
	Complete = TransactionState("complete")
	Pending  = TransactionState("pending")
	Failed   = TransactionState("failed")
	Errored  = TransactionState("errored")
)

type (
	TransactionState string
	StatusResolver   func(transactionStatus payment.TransactionStatus) (TransactionState, error)
)

func CheckCustodianStatusResponse(transactionStatus payment.TransactionStatus) (TransactionState, error) {
	return Complete, nil
}

func CheckCustodianSubmitResponse(transactionStatus payment.TransactionStatus) (TransactionState, error) {
	return Complete, nil
}

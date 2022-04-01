package transactionstatus

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

	// Complete represents when a transaction has been successfully processed.
	Complete = State("complete")
	// Pending represents when a transaction is actively being processed.
	Pending = State("pending")
	// Failed represents when a transaction has failed to process.
	Failed = State("failed")
	// Unknown represents when a call to check status is unable to determine the current state of a transaction.
	Unknown = State("unknown")
	// Errored represents when a transaction has errored during processing.
	Errored = State("errored")
)

type (
	// State represent the state of a transaction
	State string
	// Resolver implementations of this function should resolve the state of a transaction
	Resolver func(transactionStatus payment.TransactionStatus) (State, error)
)

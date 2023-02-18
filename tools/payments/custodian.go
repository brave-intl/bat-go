package payments

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var idempotencyNamespace, _ = uuid.Parse("1286fb9f-c6ac-4e97-97a3-9fd866c95926")

// Custodian is a string identifier for a given custodian.
type Custodian string

// String implements stringer interface
func (c Custodian) String() string {
	return string(c)
}

const (
	uphold   Custodian = "uphold"
	gemini             = "gemini"
	bitflyer           = "bitflyer"
)

// custodianStats is a structure which contains total amount of bat, and total number of transactions
type custodianStats struct {
	Transactions uint64
	AmountBAT    decimal.Decimal
}

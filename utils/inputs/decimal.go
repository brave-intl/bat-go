package inputs

import "github.com/shopspring/decimal"

var (
	defaultPrecision int32 = 18
)

// Decimal holds information about a decimal number
type Decimal struct {
	*decimal.Decimal
	precision int32
}

// MarshalJSON marshals the decimal to a string with a fixed amount
func (decimal Decimal) MarshalJSON() ([]byte, error) {
	return []byte(decimal.Decimal.StringFixed(decimal.precision)), nil
}

// NewDecimal creates a new decimal
func NewDecimal(decimal *decimal.Decimal, precisions ...int32) Decimal {
	precision := defaultPrecision
	if len(precisions) > 0 {
		precision = precisions[0]
	}
	return Decimal{decimal, precision}
}

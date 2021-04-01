package inputs

import (
	"strconv"

	"github.com/shopspring/decimal"
)

var (
	defaultPrecision int32 = 18
)

// Decimal holds information about a decimal number
type Decimal struct {
	*decimal.Decimal
	precision int32
}

// MarshalJSON marshals the decimal to a string with a fixed amount
func (d *Decimal) MarshalJSON() ([]byte, error) {
	str := "0"
	if d.Decimal != nil {
		str = d.StringFixed(d.precision)
	}
	return []byte(strconv.Quote(str)), nil
}

// NewDecimal creates a new decimal
func NewDecimal(d *decimal.Decimal, precisions ...int32) Decimal {
	if d == nil {
		return *new(Decimal)
	}
	precision := defaultPrecision
	if len(precisions) > 0 {
		precision = precisions[0]
	}
	return Decimal{d, precision}
}

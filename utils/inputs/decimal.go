package inputs

import "github.com/shopspring/decimal"

type Decimal struct{ *decimal.Decimal }

func (decimal Decimal) MarshalJSON() ([]byte, error) {
	return []byte(decimal.StringFixed(18)), nil
}

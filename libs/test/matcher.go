package test

import (
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/shopspring/decimal"
)

// DecEq returns a matcher that matches a decimal.Decimal of equal value.
func DecEq(x decimal.Decimal) gomock.Matcher { return decMatcher{x} }

type decMatcher struct {
	x decimal.Decimal
}

func (e decMatcher) Matches(x interface{}) bool {
	switch v := x.(type) {
	case decimal.Decimal:
		return e.x.Equals(v)
	default:
		return false
	}
}

func (e decMatcher) String() string {
	return fmt.Sprintf("is equal to %v", e.x)
}

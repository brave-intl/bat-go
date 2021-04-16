package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// AccountEarnings holds results from querying account earnings
type AccountEarnings struct {
	Channel   Channel         `json:"channel" db:"channel"`
	Earnings  decimal.Decimal `json:"earnings" db:"earnings"`
	AccountID string          `json:"account_id" db:"account_id"`
}

// AccountSettlementEarnings holds results from querying account earnings
type AccountSettlementEarnings struct {
	Channel   Channel         `json:"channel" db:"channel"`
	Paid      decimal.Decimal `json:"paid" db:"paid"`
	AccountID string          `json:"account_id" db:"account_id"`
}

// AccountEarningsOptions receives all options pertaining to account earnings calculations
type AccountEarningsOptions struct {
	Type      string
	Ascending bool
	Limit     int
}

// AccountSettlementEarningsOptions receives all options pertaining to account settlement earnings calculations
type AccountSettlementEarningsOptions struct {
	Type      string
	Ascending bool
	Limit     int
	StartDate *time.Time
	UntilDate *time.Time
}

// Balance holds information about an account id's balance
type Balance struct {
	AccountID string          `json:"account_id" db:"account_id"`
	Type      string          `json:"account_type" db:"account_type"`
	Balance   decimal.Decimal `json:"balance" db:"balance"`
}

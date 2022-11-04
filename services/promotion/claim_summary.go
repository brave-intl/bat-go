package promotion

import (
	"time"

	"github.com/shopspring/decimal"
)

// ClaimSummary outlines the state of a wallet's claims
type ClaimSummary struct {
	Amount    decimal.Decimal `json:"amount" db:"amount"`
	Earnings  decimal.Decimal `json:"earnings" db:"earnings"`
	LastClaim time.Time       `json:"lastClaim" db:"last_claim"`
	Type      string          `json:"type" db:"type"`
}

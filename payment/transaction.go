package payment

import (
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Transaction includes information about a particular order. Status can be pending, failure, completed, or error.
type Transaction struct {
	ID                    uuid.UUID       `json:"id" db:"id"`
	OrderID               uuid.UUID       `json:"order_id" db:"order_id"`
	CreatedAt             time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt             time.Time       `json:"updatedAt" db:"updated_at"`
	ExternalTransactionID string          `json:"external_transaction_id" db:"external_transaction_id"`
	Status                string          `json:"status" db:"status"`
	Currency              string          `json:"currency" db:"currency"`
	Kind                  string          `json:"kind" db:"kind"`
	Amount                decimal.Decimal `json:"totalPrice" db:"total_price"`
}

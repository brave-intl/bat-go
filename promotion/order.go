package promotion

import (
	"database/sql"
	"time"

	"github.com/brave-intl/bat-go/payment"
	"github.com/brave-intl/bat-go/utils/datastore"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Delete this file once the issue is completed
// https://github.com/brave-intl/bat-go/issues/263

// Order includes information about a particular order
type Order struct {
	ID                    uuid.UUID            `json:"id" db:"id"`
	CreatedAt             time.Time            `json:"createdAt" db:"created_at"`
	Currency              string               `json:"currency" db:"currency"`
	UpdatedAt             time.Time            `json:"updatedAt" db:"updated_at"`
	TotalPrice            decimal.Decimal      `json:"totalPrice" db:"total_price"`
	MerchantID            string               `json:"merchantId" db:"merchant_id"`
	Location              datastore.NullString `json:"location" db:"location"`
	Status                string               `json:"status" db:"status"`
	Items                 []OrderItem          `json:"items"`
	AllowedPaymentMethods payment.Methods      `json:"allowedPaymentMethods" db:"allowed_payment_methods"`
	Metadata              datastore.Metadata   `json:"metadata" db:"metadata"`
	LastPaidAt            sql.NullTime         `json:"last_paid_at" db:"last_paid_at"`
	ExpiresAt             sql.NullTime         `json:"expires_at" db:"expires_at"`
	ValidFor              *time.Duration       `json:"valid_for" db:"valid_for"`
}

// OrderItem includes information about a particular order item
type OrderItem struct {
	ID             uuid.UUID            `json:"id" db:"id"`
	OrderID        uuid.UUID            `json:"orderId" db:"order_id"`
	SKU            string               `json:"sku" db:"sku"`
	CreatedAt      *time.Time           `json:"createdAt" db:"created_at"`
	UpdatedAt      *time.Time           `json:"updatedAt" db:"updated_at"`
	Currency       string               `json:"currency" db:"currency"`
	Quantity       int                  `json:"quantity" db:"quantity"`
	Price          decimal.Decimal      `json:"price" db:"price"`
	Subtotal       decimal.Decimal      `json:"subtotal"`
	Location       datastore.NullString `json:"location" db:"location"`
	Description    datastore.NullString `json:"description" db:"description"`
	CredentialType string               `json:"credentialType" db:"credential_type"`
	ValidFor       *time.Duration       `json:"valid_for" db:"valid_for"`
	Metadata       datastore.Metadata   `json:"metadata" db:"metadata"`
}

// IsPaid returns true if the order is paid
func (order Order) IsPaid() bool {
	return order.Status == "paid"
}

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
	Amount                decimal.Decimal `json:"amount" db:"amount"`
}

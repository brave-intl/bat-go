package outputs

import (
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Meta - generic api output metadata
type Meta struct {
	Status  string                 `json:"status,omitempty"`
	Message string                 `json:"message,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// Custodian - generic custodian output data
type Custodian struct {
	Provider           string `json:"provider,omitempty" db:"user_deposit_account_provider"`
	DepositDestination string `json:"deposit_destination,omitempty" db:"user_deposit_destination"`
}

// PromotionDrained - generic custodian output data
type PromotionDrained struct {
	PromotionID   *uuid.UUID      `json:"promotion_id,omitempty" db:"promotion_id"`
	TransactionID *uuid.UUID      `json:"transaction_id,omitempty" db:"transaction_id"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	State         *string         `json:"state,omitempty" db:"state"`
	ErrCode       *string         `json:"errcode,omitempty" db:"errcode"`
	Value         decimal.Decimal `json:"value,omitempty" db:"value"`
}

// CustodianDrain - representation of a drain job
type CustodianDrain struct {
	BatchID           uuid.UUID          `json:"batch_id"`
	Custodian         Custodian          `json:"custodian,omitempty"`
	PromotionsDrained []PromotionDrained `json:"promotions_drained,omitempty"`
	Value             decimal.Decimal    `json:"value"`
}

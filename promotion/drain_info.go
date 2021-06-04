package promotion

import (
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// DrainInfo - generic custodian output data
type DrainInfo struct {
	PromotionID   *uuid.UUID      `json:"promotion_id,omitempty" db:"promotion_id"`
	TransactionID *uuid.UUID      `json:"transaction_id,omitempty" db:"transaction_id"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	State         *string         `json:"state,omitempty" db:"state"`
	ErrCode       *string         `json:"errcode,omitempty" db:"errcode"`
	Value         decimal.Decimal `json:"value,omitempty" db:"value"`
}

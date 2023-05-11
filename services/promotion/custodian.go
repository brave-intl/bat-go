package promotion

import (
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Custodian - generic custodian output data
type Custodian struct {
	Provider           string `json:"provider,omitempty" db:"user_deposit_account_provider"`
	DepositDestination string `json:"deposit_destination,omitempty" db:"user_deposit_destination"`
}

// CustodianDrain - representation of a drain job
type CustodianDrain struct {
	BatchID           uuid.UUID       `json:"batch_id"`
	Custodian         Custodian       `json:"custodian,omitempty"`
	PromotionsDrained []DrainInfo     `json:"promotions_drained,omitempty"`
	Value             decimal.Decimal `json:"value"`
}

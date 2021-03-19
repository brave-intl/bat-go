package grant

import (
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/wallet"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Grant - a "check" good for the amount inscribed, redeemable between maturityTime and expiryTime
type Grant struct {
	AltCurrency       *altcurrency.AltCurrency `json:"altcurrency" valid:"-"`
	GrantID           uuid.UUID                `json:"grantId" valid:"uuid" db:"id"`
	Probi             decimal.Decimal          `json:"probi" valid:"uuid"`
	PromotionID       uuid.UUID                `json:"promotionId" valid:"uuid" db:"promotion_id"`
	MaturityTimestamp int64                    `json:"maturityTime" valid:"-"`
	ExpiryTimestamp   int64                    `json:"expiryTime" valid:"uuid"`
	Type              string                   `json:"type,omitempty" valid:"-" db:"promotion_type"`
	ProviderID        *uuid.UUID               `json:"providerId,omitempty" valid:"uuid"`
}

// ByExpiryTimestamp implements sort.Interface for []Grant based on the ExpiryTimestamp field.
type ByExpiryTimestamp []Grant

func (a ByExpiryTimestamp) Len() int           { return len(a) }
func (a ByExpiryTimestamp) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByExpiryTimestamp) Less(i, j int) bool { return a[i].ExpiryTimestamp < a[j].ExpiryTimestamp }

// GetGrantsOrderedByExpiry returns ordered grant claims for a wallet with optional promotionType filter
func (service *Service) GetGrantsOrderedByExpiry(wallet wallet.Info, promotionType string) ([]Grant, error) {
	return service.ReadableDatastore().GetGrantsOrderedByExpiry(wallet, promotionType)
}

package promotion

import (
	"context"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Promotion includes information about a particular promotion
type Promotion struct {
	ID                  uuid.UUID       `json:"id" db:"id"`
	CreatedAt           time.Time       `json:"createdAt" db:"created_at"`
	ExpiresAt           time.Time       `json:"expiresAt" db:"expires_at"`
	Version             int             `json:"version" db:"version"`
	SuggestionsPerGrant int             `json:"suggestionsPerGrant" db:"suggestions_per_grant"`
	ApproximateValue    decimal.Decimal `json:"approximateValue" db:"approximate_value"`
	Type                string          `json:"type" db:"promotion_type"`
	RemainingGrants     int             `json:"-" db:"remaining_grants"`
	Active              bool            `json:"-" db:"active"`
	Available           bool            `json:"available" db:"available"`
	Platform            string          `json:"platform" db:"platform"`
	PublicKeys          JSONStringArray `json:"publicKeys" db:"public_keys"`
	LegacyClaimed       bool            `json:"legacyClaimed" db:"legacy_claimed"`
	//ClaimableUntil      time.Time
}

// CredentialValue returns the approximate value of a credential
func (promotion *Promotion) CredentialValue() decimal.Decimal {
	return promotion.ApproximateValue.Div(decimal.New(int64(promotion.SuggestionsPerGrant), 0))
}

// GetAvailablePromotions first tries to look up the wallet and then retrieves available promotions
func (service *Service) GetAvailablePromotions(
	ctx context.Context,
	walletID *uuid.UUID,
	platform string,
	legacy bool,
) (*[]Promotion, error) {
	if walletID != nil {
		wallet, err := service.GetOrCreateWallet(ctx, *walletID)
		if err != nil {
			return nil, err
		}
		if wallet == nil {
			return nil, nil
		}
		promos, err := service.datastore.GetAvailablePromotionsForWallet(wallet, platform, legacy)
		return &promos, err
	}
	promos, err := service.datastore.GetAvailablePromotions(platform, legacy)
	return &promos, err
}

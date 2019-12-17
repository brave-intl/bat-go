package promotion

import (
	"context"
	"time"

	"github.com/brave-intl/bat-go/utils/logging"
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

// Filter promotions to all that satisfy the function passed
func Filter(orig []Promotion, f func(Promotion) bool) []Promotion {
	promos := make([]Promotion, 0)
	for _, p := range orig {
		if f(p) {
			promos = append(promos, p)
		}
	}
	return promos
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
	migrate bool,
) (*[]Promotion, error) {
	if walletID != nil {
		logging.AddWalletIDToContext(ctx, *walletID)

		wallet, err := service.GetOrCreateWallet(ctx, *walletID)
		if err != nil {
			return nil, err
		}
		if wallet == nil {
			return nil, nil
		}

		promos, err := service.ReadableDatastore().GetAvailablePromotionsForWallet(wallet, platform, legacy)
		if err != nil {
			return nil, err
		}

		if !migrate {
			promos = Filter(promos, func(p Promotion) bool { return !p.LegacyClaimed })
		}
		return &promos, nil
	}
	promos, err := service.ReadableDatastore().GetAvailablePromotions(platform, legacy)
	return &promos, err
}

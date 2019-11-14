package promotion

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var walletCooldown int64

func init() {
	env := os.Getenv("WALLET_COOLDOWN_SECONDS")
	walletCooldownSeconds, err := strconv.ParseInt(env, 10, 64)
	if err != nil {
		panic("env: WALLET_COOLDOWN_SECONDS must be a number")
	}
	walletCooldown = walletCooldownSeconds
}

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
		if int64(time.Since(time.Now()).Seconds()) < walletCooldown {
			return nil, errors.New("promotions not available")
		}
		promos, err := service.datastore.GetAvailablePromotionsForWallet(wallet, platform, legacy)
		return &promos, err
	}
	promos, err := service.datastore.GetAvailablePromotions(platform, legacy)
	return &promos, err
}

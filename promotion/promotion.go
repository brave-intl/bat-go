package promotion

import (
	"context"
	"time"

	"github.com/brave-intl/bat-go/utils/jsonutils"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var (
	defaultVoteValue  = decimal.NewFromFloat(0.25)
	promotionGetCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "promotion_get_count",
			Help: "a count of the number of times the promotions were collected",
		},
		[]string{"filter", "migrate"},
	)
	promotionExposureCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "promotion_exposure_count",
			Help: "a count of the number of times a single promotion was exposed to clients",
		},
		[]string{"id"},
	)
)

func init() {
	prometheus.MustRegister(
		promotionGetCount,
		promotionExposureCount,
	)
}

// publicKeyFilter - filter function for dropping promotions with no public key
func publicKeyFilter(p Promotion) bool {
	return len(p.PublicKeys) > 0
}

// Promotion includes information about a particular promotion
type Promotion struct {
	ID                  uuid.UUID                 `json:"id" db:"id"`
	CreatedAt           time.Time                 `json:"createdAt" db:"created_at"`
	ExpiresAt           time.Time                 `json:"expiresAt" db:"expires_at"`
	Version             int                       `json:"version" db:"version"`
	SuggestionsPerGrant int                       `json:"suggestionsPerGrant" db:"suggestions_per_grant"`
	ApproximateValue    decimal.Decimal           `json:"approximateValue" db:"approximate_value"`
	Type                string                    `json:"type" db:"promotion_type"`
	RemainingGrants     int                       `json:"-" db:"remaining_grants"`
	Active              bool                      `json:"-" db:"active"`
	Available           bool                      `json:"available" db:"available"`
	Platform            string                    `json:"platform" db:"platform"`
	PublicKeys          jsonutils.JSONStringArray `json:"publicKeys" db:"public_keys"`
	LegacyClaimed       bool                      `json:"legacyClaimed" db:"legacy_claimed"`
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
	migrate bool,
) (*[]Promotion, error) {
	if walletID != nil {
		logging.AddWalletIDToContext(ctx, *walletID)

		wallet, err := service.wallet.GetOrCreateWallet(ctx, *walletID)
		if err != nil {
			return nil, err
		}
		if wallet == nil {
			return nil, nil
		}

		promos, err := service.ReadableDatastore().GetAvailablePromotionsForWallet(wallet, platform)
		if err != nil {
			return nil, err
		}

		// Quick hack FIXME
		for i := 0; i < len(promos); i++ {
			promos[i].ApproximateValue = decimal.New(int64(promos[i].SuggestionsPerGrant), 0).Mul(defaultVoteValue)
		}

		if !migrate {
			promos = Filter(promos, func(p Promotion) bool { return !p.LegacyClaimed })
		}

		promos = Filter(promos, publicKeyFilter)

		return &promos, nil
	}
	promos, err := service.ReadableDatastore().GetAvailablePromotions(platform)
	promos = Filter(promos, publicKeyFilter)
	return &promos, err
}

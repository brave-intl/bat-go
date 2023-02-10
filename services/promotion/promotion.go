package promotion

import (
	"context"
	"time"

	"github.com/brave-intl/bat-go/libs/jsonutils"
	"github.com/brave-intl/bat-go/libs/logging"
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
	// register our metrics with prometheus
	if err := prometheus.Register(promotionGetCount); err != nil {
		if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
			promotionGetCount = ae.ExistingCollector.(*prometheus.CounterVec)
		}
	}

	if err := prometheus.Register(promotionExposureCount); err != nil {
		if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
			promotionExposureCount = ae.ExistingCollector.(*prometheus.CounterVec)
		}
	}
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
	// warning, legacy claimed is not defined in promotions, but rather as a claim attribute
	LegacyClaimed          bool       `json:"legacyClaimed" db:"legacy_claimed"`
	ClaimableUntil         time.Time  `json:"claimableUntil" db:"claimable_until"`
	ClaimableUntilOverride *time.Time `json:"-" db:"claimable_until_override"`
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
	cv := promotion.ApproximateValue.Div(decimal.New(int64(promotion.SuggestionsPerGrant), 0))
	if !cv.Sub(decimal.NewFromFloat(0.25)).IsZero() {
		panic("wrong credential value calculated for promotion")
	}
	return cv
}

// Claimable checks whether the promotion can be claimed
func (promotion *Promotion) Claimable(overrideAutoExpiry bool) bool {
	// always refuse expired promotions
	if promotion.Expired() {
		return false
	}
	// override auto expiry (in legacy claimed case as example)
	if overrideAutoExpiry {
		return true
	}
	// expire grants created 3 months ago
	if promotion.CreatedAt.Before(time.Now().AddDate(0, -3, 0)) {
		return false
	}
	return true
}

// Expired check if now is after the expires_at time
func (promotion *Promotion) Expired() bool {
	return promotion.ExpiresAt.Before(time.Now())
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

		wallet, err := service.wallet.GetWallet(ctx, *walletID)
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

		return &promos, nil
	}
	promos, err := service.ReadableDatastore().GetAvailablePromotions(platform)
	return &promos, err
}

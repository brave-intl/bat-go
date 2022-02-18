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
	promotionV2GetCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "promotion_v2_get_count",
			Help: "a count of the number of times the swap promotions were collected",
		},
		[]string{"platform"},
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
	err := prometheus.Register(promotionGetCount)
	if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
		promotionGetCount = ae.ExistingCollector.(*prometheus.CounterVec)
	}

	err = prometheus.Register(promotionExposureCount)
	if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
		promotionExposureCount = ae.ExistingCollector.(*prometheus.CounterVec)
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
	Platform            string                    `json:"platform" db:"platform"`
	PublicKeys          jsonutils.JSONStringArray `json:"publicKeys" db:"public_keys"`
	Available           bool                      `json:"available" db:"available"`
	AutoClaim           *bool                     `json:"-" db:"auto_claim"`
	SkipCaptcha         *bool                     `json:"-" db:"skip_captcha"`
	NumSuggestions      *int                      `json:"-" db:"num_suggestions"`
	// warning, legacy claimed is not defined in promotions, but rather as a claim attribute
	LegacyClaimed  bool      `json:"legacyClaimed" db:"legacy_claimed"`
	ClaimableUntil time.Time `json:"claimableUntil" db:"claimable_until"`
}

// PromotionV2 includes a new format for information about a particular promotion
//nolint
type PromotionV2 struct {
	ID               uuid.UUID                 `json:"id"`
	ExpiresAt        time.Time                 `json:"expiresAt"`
	Version          int                       `json:"version"`
	NumSuggestions   *int                      `json:"numSuggestions,omitempty"`
	ApproximateValue decimal.Decimal           `json:"approximateValue"`
	Type             string                    `json:"type"`
	AutoClaim        *bool                     `json:"autoClaim,omitempty"`
	SkipCaptcha      *bool                     `json:"skipCaptcha,omitempty"`
	Available        bool                      `json:"available"`
	PublicKeys       jsonutils.JSONStringArray `json:"publicKeys"`
}

// PromotionToV2 is a helper function to go from Promotion to PromotionV2
//nolint
func PromotionToV2(promo Promotion) PromotionV2 {
	return PromotionV2{ID: promo.ID, ExpiresAt: promo.ExpiresAt, Version: promo.Version, NumSuggestions: promo.NumSuggestions, ApproximateValue: promo.ApproximateValue, Type: promo.Type, AutoClaim: promo.AutoClaim, SkipCaptcha: promo.SkipCaptcha, Available: promo.Available, PublicKeys: promo.PublicKeys}
}

// PromotionsToV2 is a helper function to go from []Promotion to []PromotionV2
func PromotionsToV2(promos []Promotion) []PromotionV2 {
	promotionsv2 := []PromotionV2{}
	for _, promotion := range promos {
		promotionsv2 = append(promotionsv2, PromotionToV2(promotion))
	}

	return promotionsv2
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

// Claimable checks whether the promotion can be claimed
func (promotion *Promotion) Claimable(overrideAutoExpiry bool) bool {
	// manually disallow claims
	if !promotion.Active {
		return false
	}
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

// GetAvailablePromotionsV2 first tries to look up the wallet and then retrieves available promotions
func (service *Service) GetAvailablePromotionsV2(
	ctx context.Context,
	walletID *uuid.UUID,
	platform string,
) (*[]PromotionV2, error) {
	if walletID != nil {
		logging.AddWalletIDToContext(ctx, *walletID)

		wallet, err := service.wallet.GetWallet(ctx, *walletID)
		if err != nil {
			return nil, err
		}
		if wallet == nil {
			return nil, nil
		}

		promos, err := service.ReadableDatastore().GetAvailablePromotionsV2ForWallet(wallet, platform)
		if err != nil {
			return nil, err
		}

		return &promos, nil
	}
	promos, err := service.ReadableDatastore().GetAvailablePromotionsV2(platform)
	return &promos, err
}

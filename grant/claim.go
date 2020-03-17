package grant

import (
	"context"
	"fmt"

	promotion "github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// ClaimRequest is a request to claim a grant
type ClaimRequest struct {
	PromotionID uuid.UUID   `json:"promotionId" valid:"-"`
	WalletInfo  wallet.Info `json:"wallet" valid:"required"`
}

// ClaimResponse includes information about the claimed grant
type ClaimResponse struct {
	ApproximateValue decimal.Decimal `json:"approximateValue" db:"approximate_value"`
}

// ClaimPromotion registers a claim on behalf of a user wallet to a particular Promotion.
func (service *Service) ClaimPromotion(ctx context.Context, wallet wallet.Info, promotionID uuid.UUID) (*promotion.Claim, error) {
	err := service.datastore.UpsertWallet(&wallet)
	if err != nil {
		return nil, fmt.Errorf("error saving wallet: %w", err)
	}

	promotion, err := service.datastore.GetPromotion(promotionID)
	if err != nil {
		return nil, fmt.Errorf("could not find promotion: %w", err)
	}

	// No reputation check as this endpoint requires authorization

	claim, err := service.datastore.ClaimPromotionForWallet(promotion, &wallet)
	if err != nil {
		log.Error().Msg("Attempt to claim previously claimed grant!")
		return nil, err
	}
	claimedGrantsCounter.With(prometheus.Labels{}).Inc()

	return claim, nil
}

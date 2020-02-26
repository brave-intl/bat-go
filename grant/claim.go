package grant

import (
	"context"

	promotion "github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
)

// ClaimPromotion registers a claim on behalf of a user wallet to a particular Promotion.
func (service *Service) ClaimPromotion(ctx context.Context, wallet wallet.Info, promotionID uuid.UUID) (*promotion.Claim, error) {
	err := service.datastore.UpsertWallet(&wallet)
	if err != nil {
		return nil, errors.Wrap(err, "Error saving wallet")
	}

	promotion, err := service.datastore.GetPromotion(promotionID)
	if err != nil {
		return nil, errors.Wrap(err, "Could not find promotion")
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

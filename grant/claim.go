package grant

import (
	"context"

	promotion "github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/rs/zerolog/log"
)

// ClaimGrantWithGrantIDRequest is a request to claim a grant
type ClaimGrantWithGrantIDRequest struct {
	WalletInfo wallet.Info `json:"wallet" valid:"required"`
}

// ClaimRequest is a request to claim a grant
type ClaimRequest struct {
	PromotionID uuid.UUID   `json:"promotionId" valid:"-"`
	WalletInfo  wallet.Info `json:"wallet" valid:"required"`
}

// ClaimResponse includes information about the claimed grant
type ClaimResponse struct {
	ApproximateValue decimal.Decimal `json:"approximateValue" db:"approximate_value"`
}

// Claim registers a claim on behalf of a user wallet to a particular Grant.
// Registered claims are enforced by RedeemGrantsRequest.Verify.
func (service *Service) Claim(ctx context.Context, wallet wallet.Info, grant Grant) error {
	loggerCtx := log.Logger.WithContext(ctx)

	err := service.datastore.UpsertWallet(&wallet)
	if err != nil {
		return errors.Wrap(err, "Error saving wallet")
	}

	err = service.datastore.ClaimGrantForWallet(grant, wallet)
	if err != nil {
		log.Ctx(ctx).
			Error().
			Msg("Attempt to claim previously claimed grant!")
		return err
	}
	claimedGrantsCounter.With(prometheus.Labels{}).Inc()

	return nil
}

// ClaimPromotion registers a claim on behalf of a user wallet to a particular Promotion.
func (service *Service) ClaimPromotion(ctx context.Context, wallet wallet.Info, promotionID uuid.UUID) (*promotion.Claim, error) {
	log := lg.Log(ctx)

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
		log.Error("Attempt to claim previously claimed grant!")
		return nil, err
	}
	claimedGrantsCounter.With(prometheus.Labels{}).Inc()

	return claim, nil
}

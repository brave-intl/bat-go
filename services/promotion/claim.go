package promotion

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/jsonutils"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/getsentry/sentry-go"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Claim encapsulates a redeemed or unredeemed ("pre-registered") claim to a promotion by a wallet
type Claim struct {
	ID               uuid.UUID       `db:"id"`
	CreatedAt        time.Time       `db:"created_at"`
	PromotionID      uuid.UUID       `db:"promotion_id"`
	WalletID         uuid.UUID       `db:"wallet_id"`
	ApproximateValue decimal.Decimal `db:"approximate_value"`
	Redeemed         bool            `db:"redeemed"`
	Bonus            decimal.Decimal `db:"bonus"`
	LegacyClaimed    bool            `db:"legacy_claimed"`
	RedeemedAt       pq.NullTime     `db:"redeemed_at"`
	Drained          bool            `db:"drained"`
	DrainedAt        pq.NullTime     `db:"drained_at"`
	UpdatedAt        pq.NullTime     `db:"updated_at"`
	ClaimType        *string         `db:"claim_type"`
}

// SuggestionsNeeded calculates the number of suggestion credentials needed to fulfill the value of this claim
func (claim *Claim) SuggestionsNeeded(promotion *Promotion) (int, error) {
	if claim.PromotionID != promotion.ID {
		return 0, errors.New("incorrect promotion passed")
	}
	amount := int(claim.ApproximateValue.Mul(decimal.NewFromFloat(float64(promotion.SuggestionsPerGrant)).Div(promotion.ApproximateValue)).Round(0).IntPart())
	if amount < 1 {
		return 1, nil
	}
	return amount, nil
}

// ClaimCreds encapsulates the credentials to be signed in response to a valid claim
type ClaimCreds struct {
	ID           uuid.UUID                  `db:"claim_id"`
	IssuerID     uuid.UUID                  `db:"issuer_id"`
	BlindedCreds jsonutils.JSONStringArray  `db:"blinded_creds"`
	SignedCreds  *jsonutils.JSONStringArray `db:"signed_creds"`
	BatchProof   *string                    `db:"batch_proof"`
	PublicKey    *string                    `db:"public_key"`
	CreatedAt    pq.NullTime                `db:"created_at"`
	UpdatedAt    pq.NullTime                `db:"updated_at"`
}

func blindCredsEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	// a and b must have same values in same order
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var errClaimedDifferentBlindCreds = errors.New("blinded credentials do not match what was already claimed")

// ClaimPromotionForWallet attempts to claim the promotion on behalf of a wallet and returning the ClaimID
// It kicks off asynchronous signing of the credentials on success
func (service *Service) ClaimPromotionForWallet(
	ctx context.Context,
	promotionID uuid.UUID,
	walletID uuid.UUID,
	blindedCreds []string,
) (*uuid.UUID, error) {

	logger := logging.Logger(ctx, "ClaimPromotionForWallet")

	promotion, err := service.Datastore.GetPromotion(promotionID)
	if err != nil {
		return nil, err
	}
	if promotion == nil {
		return nil, errors.New("promotion did not exist")
	}

	wallet, err := service.wallet.Datastore.GetWallet(ctx, walletID)
	if err != nil || wallet == nil {
		return nil, errorutils.Wrap(err, "error getting wallet")
	}

	claim, err := service.Datastore.GetClaimByWalletAndPromotion(wallet, promotion)
	if err != nil {
		return nil, errorutils.Wrap(err, "error checking previous claims for wallet")
	}

	// check if we need to override the auto expiry of the promotion
	overrideAutoExpiry := false
	if claim != nil {
		overrideAutoExpiry = claim.LegacyClaimed
	}

	// check if promotion is claimable
	if !promotion.Claimable(overrideAutoExpiry) {
		return nil, &handlers.AppError{
			Message: "promotion is no longer active",
			Code:    http.StatusGone,
		}
	}

	if claim != nil {
		// get the claim credentials to check if these blinded creds were used before
		claimCreds, err := service.Datastore.GetClaimCreds(claim.ID)
		if err != nil {
			return nil, errorutils.Wrap(err, "error checking claim credentials for claims")
		}

		if claim.Redeemed {
			if claimCreds == nil {
				// there are no stored claim creds for this claim
				logger.Error().
					Str("wallet_id", walletID.String()).
					Str("claim_id", claim.ID.String()).
					Msg("nil claim credentials for claim")
				return nil, errors.New("nil claim credentials recorded")
			}

			// If this wallet already claimed and it was redeemed (legacy or into claim creds), return the claim id
			// and the claim blinded tokens are the same
			if blindCredsEq([]string(claimCreds.BlindedCreds), blindedCreds) {
				return &claim.ID, nil
			}
			return nil, errClaimedDifferentBlindCreds
		}
	}

	// check if promotion is disabled, need different behavior than Gone
	if !promotion.Active {
		return nil, &handlers.AppError{
			Message: "promotion is disabled",
			Code:    http.StatusBadRequest,
		}
	}

	// This is skipped for legacy migration path as they passed a reputation check when originally claiming
	if claim == nil || !claim.LegacyClaimed {
		walletIsReputable, err := service.reputationClient.IsWalletReputable(ctx, walletID, promotion.Platform)
		if err != nil {
			return nil, err
		}

		if !walletIsReputable {
			return nil, errors.New("insufficient wallet reputation for grant claim")
		}
	}

	cohort := "control"
	issuer, err := service.GetOrCreateIssuer(ctx, promotionID, cohort)
	if err != nil {
		return nil, err
	}

	if promotion.Type == "ads" {
		claim, err := service.Datastore.GetPreClaim(promotionID, wallet.ID)
		if err != nil {
			return nil, err
		}

		if claim == nil {
			return nil, errors.New("you cannot claim this promotion")
		}

		suggestionsNeeded, err := claim.SuggestionsNeeded(promotion)
		if err != nil {
			return nil, err
		}
		if len(blindedCreds) != suggestionsNeeded {
			return nil, errors.New("wrong number of blinded tokens included")
		}
	} else {
		if len(blindedCreds) != promotion.SuggestionsPerGrant {
			return nil, errors.New("wrong number of blinded tokens included")
		}
	}

	claim, err = service.Datastore.ClaimForWallet(promotion, issuer, wallet, jsonutils.JSONStringArray(blindedCreds))
	if err != nil {
		return nil, err
	}

	value, _ := claim.ApproximateValue.Float64()
	labels := prometheus.Labels{
		"platform": promotion.Platform,
		"type":     promotion.Type,
		"legacy":   strconv.FormatBool(claim.LegacyClaimed),
	}
	countGrantsClaimedTotal.With(labels).Inc()
	countGrantsClaimedBatTotal.With(labels).Add(value)

	go func() {
		defer middleware.ConcurrentGoRoutines.With(
			prometheus.Labels{
				"method": "ClaimJob",
			}).Dec()

		middleware.ConcurrentGoRoutines.With(
			prometheus.Labels{
				"method": "ClaimJob",
			}).Inc()
		_, err := service.RunNextClaimJob(ctx)
		if err != nil {
			sentry.CaptureException(err)
		}
	}()

	return &claim.ID, nil
}

// ClaimWorker attempts to work on a claim job by signing the blinded credentials of the client
type ClaimWorker interface {
	SignClaimCreds(ctx context.Context, claimID uuid.UUID, issuer Issuer, blindedCreds []string) (*ClaimCreds, error)
}

// SignClaimCreds signs the blinded credentials
func (service *Service) SignClaimCreds(ctx context.Context, claimID uuid.UUID, issuer Issuer, blindedCreds []string) (*ClaimCreds, error) {
	resp, err := service.cbClient.SignCredentials(ctx, issuer.Name(), blindedCreds)
	if err != nil {
		return nil, err
	}

	signedTokens := jsonutils.JSONStringArray(resp.SignedTokens)

	creds := &ClaimCreds{
		ID:           claimID,
		BlindedCreds: blindedCreds,
		SignedCreds:  &signedTokens,
		BatchProof:   &resp.BatchProof,
		PublicKey:    &issuer.PublicKey,
	}

	return creds, nil
}

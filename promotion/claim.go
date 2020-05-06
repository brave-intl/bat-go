package promotion

import (
	"context"
	"errors"
	"strconv"
	"time"

	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/jsonutils"
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
}

// ClaimPromotionForWallet attempts to claim the promotion on behalf of a wallet and returning the ClaimID
// It kicks off asynchronous signing of the credentials on success
func (service *Service) ClaimPromotionForWallet(
	ctx context.Context,
	promotionID uuid.UUID,
	walletID uuid.UUID,
	blindedCreds []string,
) (*uuid.UUID, error) {
	promotion, err := service.datastore.GetPromotion(promotionID)
	if err != nil {
		return nil, err
	}
	if promotion == nil {
		return nil, errors.New("promotion did not exist")
	}

	wallet, err := service.datastore.GetWallet(walletID)
	if err != nil || wallet == nil {
		return nil, errorutils.Wrap(err, "error getting wallet")
	}

	claim, err := service.datastore.GetClaimByWalletAndPromotion(wallet, promotion)
	if err != nil {
		return nil, errorutils.Wrap(err, "error checking previous claims for wallet")
	}

	// If this wallet already claimed and it was redeemed (legacy or into claim creds), return the claim id
	if claim != nil && claim.Redeemed {
		return &claim.ID, nil
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
		claim, err := service.datastore.GetPreClaim(promotionID, wallet.ID)
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

	claim, err = service.datastore.ClaimForWallet(promotion, issuer, wallet, jsonutils.JSONStringArray(blindedCreds))
	if err != nil {
		return nil, err
	}

	if claim.LegacyClaimed {
		err = service.balanceClient.InvalidateBalance(ctx, walletID)
		if err != nil {
			sentry.CaptureException(err)
		}
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

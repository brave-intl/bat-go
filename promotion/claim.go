package promotion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/brave-intl/bat-go/middleware"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/jsonutils"
	"github.com/brave-intl/bat-go/utils/validators"
	"github.com/getsentry/sentry-go"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	segmentKafka "github.com/segmentio/kafka-go"
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
	AddressID        *string         `db:"address_id"`
	TransactionKey   *string         `db:"transaction_key"`
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
	for _, v := range a {
		for _, vv := range b {
			if v != vv {
				return false
			}
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

		// If this wallet already claimed and it was redeemed (legacy or into claim creds), return the claim id
		// and the claim blinded tokens are the same
		if claim.Redeemed && blindCredsEq([]string(claimCreds.BlindedCreds), blindedCreds) {
			return &claim.ID, nil
		}

		// if blinded creds do not match prior attempt, return error
		if claim.Redeemed && !blindCredsEq([]string(claimCreds.BlindedCreds), blindedCreds) {
			return nil, errClaimedDifferentBlindCreds
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

// SwapRewardGrant encapsulates the information from a reward grant sent to kafka
type SwapRewardGrant struct {
	AddressID      string
	PromotionID    uuid.UUID
	TransactionKey uuid.UUID
	RewardAmount   decimal.Decimal
}

// SwapRewardsWorker - gets reward grant information
type SwapRewardsWorker interface {
	FetchRewardsGrants(ctx context.Context) (*SwapRewardGrant, *segmentKafka.Message, error)
}

// FetchRewardsGrants - retrieves grant from topic
func (service *Service) FetchRewardsGrants(ctx context.Context) (*SwapRewardGrant, *segmentKafka.Message, error) {

	message, err := service.kafkaGrantRewardsReader.FetchMessage(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("read message: error reading kafka message %w", err)
	}

	codec, ok := service.codecs["rewardsTopic"]
	if !ok {
		return nil, nil, fmt.Errorf("read message: could not find codec %s", rewardsTopic)
	}

	native, _, err := codec.NativeFromBinary(message.Value)
	if err != nil {
		return nil, nil, fmt.Errorf("read message: error could not decode naitve from binary %w", err)
	}

	textual, err := codec.TextualFromNative(nil, native)
	if err != nil {
		return nil, nil, fmt.Errorf("read message: error could not decode textual from native %w", err)
	}

	var grantRewardsEvent GrantRewardsEvent
	err = json.Unmarshal(textual, &grantRewardsEvent)
	if err != nil {
		return nil, nil, fmt.Errorf("read message: error could not decode json from textual %w", err)
	}

	if !validators.IsETHAddress(grantRewardsEvent.AddressID) {
		return nil, nil, fmt.Errorf("read message: error could not decode adressID %s", grantRewardsEvent.AddressID)
	}

	promotionID := uuid.FromStringOrNil(grantRewardsEvent.PromotionID)
	if promotionID == uuid.Nil {
		return nil, nil, fmt.Errorf("read message: error could not decode PromotionID %s", grantRewardsEvent.PromotionID)
	}

	transactionKey := uuid.FromStringOrNil(grantRewardsEvent.TransactionKey)
	if transactionKey == uuid.Nil {
		return nil, nil, fmt.Errorf("read message: error could not decode TransactionKey %s", grantRewardsEvent.TransactionKey)
	}

	rewardAmount, err := decimal.NewFromString(grantRewardsEvent.RewardAmount)
	if err != nil {
		return nil, nil, fmt.Errorf("read message: error could not decode RewardAmount %s", grantRewardsEvent.RewardAmount)
	}

	grant := &SwapRewardGrant{
		AddressID:      grantRewardsEvent.AddressID,
		PromotionID:    promotionID,
		TransactionKey: transactionKey,
		RewardAmount:   rewardAmount,
	}

	return grant, &message, nil
}

package promotion

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"time"

	raven "github.com/getsentry/raven-go"
	"github.com/jmoiron/sqlx/types"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// JSONStringArray is a wrapper around a string array for sql serialization purposes
type JSONStringArray []string

// Scan the src sql type into the passed JSONStringArray
func (arr *JSONStringArray) Scan(src interface{}) error {
	var jt types.JSONText

	if err := jt.Scan(src); err != nil {
		return err
	}

	if err := jt.Unmarshal(arr); err != nil {
		return err
	}

	return nil
}

// Value the driver.Value representation
func (arr *JSONStringArray) Value() (driver.Value, error) {
	var jt types.JSONText

	data, err := json.Marshal((*[]string)(arr))
	if err != nil {
		return nil, err
	}

	if err := jt.UnmarshalJSON(data); err != nil {
		return nil, err
	}

	return jt.Value()
}

// MarshalJSON returns the JSON representation
func (arr *JSONStringArray) MarshalJSON() ([]byte, error) {
	return json.Marshal((*[]string)(arr))
}

// UnmarshalJSON sets the passed JSONStringArray to the value deserialized from JSON
func (arr *JSONStringArray) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, (*[]string)(arr)); err != nil {
		return err
	}

	return nil
}

// Claim encapsulates a redeemed or unredeemed ("pre-registered") claim to a promotion by a wallet
type Claim struct {
	ID               uuid.UUID       `db:"id"`
	CreatedAt        time.Time       `db:"created_at"`
	PromotionID      uuid.UUID       `db:"promotion_id"`
	WalletID         uuid.UUID       `db:"wallet_id"`
	ApproximateValue decimal.Decimal `db:"approximate_value"`
	Redeemed         bool            `db:"redeemed"`
	Bonus            decimal.Decimal `db:"bonus"`
	LegacyClaimed    string          `db:"legacy_claimed"`
}

// ClaimCreds encapsulates the credentials to be signed in response to a valid claim
type ClaimCreds struct {
	ID           uuid.UUID        `db:"claim_id"`
	IssuerID     uuid.UUID        `db:"issuer_id"`
	BlindedCreds JSONStringArray  `db:"blinded_creds"`
	SignedCreds  *JSONStringArray `db:"signed_creds"`
	BatchProof   *string          `db:"batch_proof"`
	PublicKey    *string          `db:"public_key"`
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
		return nil, errors.Wrap(err, "Error getting wallet")
	}

	claim, err := service.datastore.GetClaimByWalletAndPromotion(wallet, promotion)
	if err != nil {
		return nil, errors.Wrap(err, "Error checking previous claims for wallet")
	}

	// If this wallet already claimed, return the previously claimed promotion
	if claim != nil {
		return &claim.ID, nil
	}

	walletIsReputable, err := service.reputationClient.IsWalletReputable(ctx, walletID)
	if err != nil {
		return nil, err
	}

	if !walletIsReputable {
		return nil, errors.New("Insufficient wallet reputation for grant claim")
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

		suggestionsNeeded := int(claim.ApproximateValue.Mul(decimal.NewFromFloat(float64(promotion.SuggestionsPerGrant)).Div(promotion.ApproximateValue)).IntPart())
		if len(blindedCreds) != suggestionsNeeded {
			return nil, errors.New("wrong number of blinded tokens included")
		}
	} else {
		if len(blindedCreds) != promotion.SuggestionsPerGrant {
			return nil, errors.New("wrong number of blinded tokens included")
		}
	}

	claim, err = service.datastore.ClaimForWallet(promotion, issuer, wallet, JSONStringArray(blindedCreds))
	if err != nil {
		return nil, err
	}

	go func() {
		_, err := service.RunNextClaimJob(ctx)
		if err != nil {
			raven.CaptureErrorAndWait(err, nil)
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

	signedTokens := JSONStringArray(resp.SignedTokens)

	creds := &ClaimCreds{
		ID:           claimID,
		BlindedCreds: blindedCreds,
		SignedCreds:  &signedTokens,
		BatchProof:   &resp.BatchProof,
		PublicKey:    &issuer.PublicKey,
	}

	return creds, nil
}

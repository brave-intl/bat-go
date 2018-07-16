package grant

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/brave-intl/bat-go/datastore"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	jose "gopkg.in/square/go-jose.v2"
)

// Grant - a "check" good for the amount inscribed, redeemable between maturityTime and expiryTime
type Grant struct {
	AltCurrency       *altcurrency.AltCurrency `json:"altcurrency"`
	GrantID           uuid.UUID                `json:"grantId"`
	Probi             decimal.Decimal          `json:"probi"`
	PromotionID       uuid.UUID                `json:"promotionId"`
	MaturityTimestamp int64                    `json:"maturityTime"`
	ExpiryTimestamp   int64                    `json:"expiryTime"`
}

// ByProbi implements sort.Interface for []Grant based on the Probi field.
type ByProbi []Grant

func (a ByProbi) Len() int           { return len(a) }
func (a ByProbi) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByProbi) Less(i, j int) bool { return a[i].Probi.LessThan(a[j].Probi) }

// CreateGrants creates the specified number of grants and returns them in compact JWS serialization
func CreateGrants(
	signer jose.Signer,
	promotionUUID uuid.UUID,
	grantCount uint,
	altCurrency altcurrency.AltCurrency,
	value float64,
	maturityDate time.Time,
	expiryDate time.Time,
) []string {
	grants := make([]string, 0, grantCount)
	for i := 0; i < int(grantCount); i++ {
		var grant Grant
		grant.AltCurrency = &altCurrency
		grant.GrantID = uuid.NewV4()
		grant.Probi = altCurrency.ToProbi(decimal.NewFromFloat(value))
		grant.PromotionID = promotionUUID
		grant.MaturityTimestamp = maturityDate.Unix()
		grant.ExpiryTimestamp = expiryDate.Unix()

		serializedGrant, err := json.Marshal(grant)
		if err != nil {
			log.Fatalln(err)
		}
		jws, err := signer.Sign(serializedGrant)
		if err != nil {
			log.Fatalln(err)
		}
		serializedJWS, err := jws.CompactSerialize()
		if err != nil {
			log.Fatalln(err)
		}
		grants = append(grants, serializedJWS)
	}
	return grants
}

// FromCompactJWS parses a Grant object from one stored using compact JWS serialization.
// It returns a pointer to the parsed Grant object if it is valid and signed by the grantPublicKey.
// Otherwise an error is returned.
func FromCompactJWS(s string) (*Grant, error) {
	jws, err := jose.ParseSigned(s)
	if err != nil {
		return nil, err
	}
	for _, sig := range jws.Signatures {
		if sig.Header.Algorithm != "EdDSA" {
			return nil, errors.New("Error unsupported JWS algorithm")
		}
	}
	jwk := jose.JSONWebKey{Key: grantPublicKey}
	grantBytes, err := jws.Verify(jwk)
	if err != nil {
		return nil, err
	}

	var grant Grant
	err = json.Unmarshal(grantBytes, &grant)
	if err != nil {
		return nil, err
	}
	return &grant, nil
}

// DecodeGrants checks signatures decodes a list of grants in compact jws form
func DecodeGrants(grants []string) ([]Grant, error) {
	// 1. Check grant signatures and decode
	decoded := make([]Grant, 0, len(grants))
	for _, grantJWS := range grants {
		grant, err := FromCompactJWS(grantJWS)
		if err != nil {
			return nil, err
		}
		decoded = append(decoded, *grant)
	}
	return decoded, nil
}

// GetPromotionGrantsDatastore for tracking redemption of specific grants in a promotion
func GetPromotionGrantsDatastore(ctx context.Context, promotionID string) (datastore.SetLikeDatastore, error) {
	return datastore.GetSetDatastore(ctx, "promotion:"+promotionID+":grants")
}

// GetPromotionWalletsDatastore for tracking redemption by specific wallets in a promotion
func GetPromotionWalletsDatastore(ctx context.Context, promotionID string) (datastore.SetLikeDatastore, error) {
	return datastore.GetSetDatastore(ctx, "promotion:"+promotionID+":wallets")
}

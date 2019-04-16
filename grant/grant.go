package grant

import (
	"encoding/json"
	"errors"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/ed25519"
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
	Type              string                   `json:"type,omitempty"`
	ProviderID        *uuid.UUID               `json:"providerId,omitempty"`
}

// ByProbi implements sort.Interface for []Grant based on the Probi field.
type ByProbi []Grant

func (a ByProbi) Len() int           { return len(a) }
func (a ByProbi) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByProbi) Less(i, j int) bool { return a[i].Probi.LessThan(a[j].Probi) }

// ByExpiryTimestamp implements sort.Interface for []Grant based on the ExpiryTimestamp field.
type ByExpiryTimestamp []Grant

func (a ByExpiryTimestamp) Len() int           { return len(a) }
func (a ByExpiryTimestamp) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByExpiryTimestamp) Less(i, j int) bool { return a[i].ExpiryTimestamp < a[j].ExpiryTimestamp }

// CreateGrants creates the specified number of grants and returns them in compact JWS serialization
func CreateGrants(
	signer jose.Signer,
	template Grant,
	grantCount uint,
) ([]string, error) {
	grants := make([]string, 0, grantCount)
	for i := 0; i < int(grantCount); i++ {
		var grant Grant
		grant.AltCurrency = template.AltCurrency
		grant.GrantID = uuid.NewV4()
		grant.Probi = template.Probi
		grant.PromotionID = template.PromotionID
		grant.MaturityTimestamp = template.MaturityTimestamp
		grant.ExpiryTimestamp = template.ExpiryTimestamp
		grant.Type = template.Type
		grant.ProviderID = template.ProviderID

		serializedGrant, err := json.Marshal(grant)
		if err != nil {
			return nil, err
		}
		jws, err := signer.Sign(serializedGrant)
		if err != nil {
			return nil, err
		}
		serializedJWS, err := jws.CompactSerialize()
		if err != nil {
			return nil, err
		}
		grants = append(grants, serializedJWS)
	}
	return grants, nil
}

// FromCompactJWS parses a Grant object from one stored using compact JWS serialization.
// It returns a pointer to the parsed Grant object if it is valid and signed by pubKey.
// Otherwise an error is returned.
func FromCompactJWS(pubKey ed25519.PublicKey, s string) (*Grant, error) {
	jws, err := jose.ParseSigned(s)
	if err != nil {
		return nil, err
	}
	for _, sig := range jws.Signatures {
		if sig.Header.Algorithm != "EdDSA" {
			return nil, errors.New("Error unsupported JWS algorithm")
		}
	}
	jwk := jose.JSONWebKey{Key: pubKey}
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
func DecodeGrants(pubKey ed25519.PublicKey, grants []string) ([]Grant, error) {
	// 1. Check grant signatures and decode
	decoded := make([]Grant, 0, len(grants))
	for _, grantJWS := range grants {
		grant, err := FromCompactJWS(pubKey, grantJWS)
		if err != nil {
			return nil, err
		}
		decoded = append(decoded, *grant)
	}
	return decoded, nil
}

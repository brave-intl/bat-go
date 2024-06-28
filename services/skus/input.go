package skus

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/logging"
)

// VerifyCredentialRequestV1 includes an opaque subscription credential blob
type VerifyCredentialRequestV1 struct {
	Version      float64 `json:"version" valid:"-"`
	Type         string  `json:"type" valid:"in(single-use|time-limited|time-limited-v2)"`
	SKU          string  `json:"sku" valid:"-"`
	MerchantID   string  `json:"merchantId" valid:"-"`
	Presentation string  `json:"presentation" valid:"base64"`
}

// GetSku - implement credential interface
func (vcr *VerifyCredentialRequestV1) GetSku(ctx context.Context) string {
	return vcr.SKU
}

// GetType - implement credential interface
func (vcr *VerifyCredentialRequestV1) GetType(ctx context.Context) string {
	return vcr.Type
}

// GetMerchantID - implement credential interface
func (vcr *VerifyCredentialRequestV1) GetMerchantID(ctx context.Context) string {
	return vcr.MerchantID
}

// GetPresentation - implement credential interface
func (vcr *VerifyCredentialRequestV1) GetPresentation(ctx context.Context) string {
	return vcr.Presentation
}

// VerifyCredentialRequestV2 includes an opaque subscription credential blob
type VerifyCredentialRequestV2 struct {
	SKU              string                  `json:"sku" valid:"-"`
	MerchantID       string                  `json:"merchantId" valid:"-"`
	Credential       string                  `json:"credential" valid:"base64"`
	CredentialOpaque *VerifyCredentialOpaque `json:"-" valid:"-"`
}

// GetSku - implement credential interface
func (vcr *VerifyCredentialRequestV2) GetSku(ctx context.Context) string {
	return vcr.SKU
}

// GetType - implement credential interface
func (vcr *VerifyCredentialRequestV2) GetType(ctx context.Context) string {
	if vcr.CredentialOpaque == nil {
		return ""
	}
	return vcr.CredentialOpaque.Type
}

// GetMerchantID - implement credential interface
func (vcr *VerifyCredentialRequestV2) GetMerchantID(ctx context.Context) string {
	return vcr.MerchantID
}

// GetPresentation - implement credential interface
func (vcr *VerifyCredentialRequestV2) GetPresentation(ctx context.Context) string {
	if vcr.CredentialOpaque == nil {
		return ""
	}
	return vcr.CredentialOpaque.Presentation
}

// Decode - implement Decodable interface
func (vcr *VerifyCredentialRequestV2) Decode(ctx context.Context, data []byte) error {
	logger := logging.Logger(ctx, "VerifyCredentialRequestV2.Decode")
	logger.Debug().Msg("starting VerifyCredentialRequestV2.Decode")
	var err error

	if err := json.Unmarshal(data, vcr); err != nil {
		return fmt.Errorf("failed to json decode credential request payload: %w", err)
	}
	// decode the opaque credential
	if vcr.CredentialOpaque, err = credentialOpaqueFromString(vcr.Credential); err != nil {
		return fmt.Errorf("failed to decode opaque credential payload: %w", err)
	}
	return nil
}

// Validate - implement Validable interface
func (vcr *VerifyCredentialRequestV2) Validate(ctx context.Context) error {
	logger := logging.Logger(ctx, "VerifyCredentialRequestV2.Validate")
	var err error
	for _, v := range []interface{}{vcr, vcr.CredentialOpaque} {
		_, err = govalidator.ValidateStruct(v)
		if err != nil {
			logger.Error().Err(err).Msg("failed to validate request")
			return fmt.Errorf("failed to validate verify credential request: %w", err)
		}
	}
	return nil
}

// VerifyCredentialOpaque includes an opaque presentation blob
type VerifyCredentialOpaque struct {
	Type         string  `json:"type" valid:"in(single-use|time-limited|time-limited-v2)"`
	Version      float64 `json:"version" valid:"-"`
	Presentation string  `json:"presentation" valid:"base64"`
}

// credentialOpaqueFromString - given a base64 encoded "credential" unmarshal into a VerifyCredentialOpaque
func credentialOpaqueFromString(s string) (*VerifyCredentialOpaque, error) {
	d, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("failed to base64 decode credential payload: %w", err)
	}
	var vcp = new(VerifyCredentialOpaque)
	if err = json.Unmarshal(d, vcp); err != nil {
		return nil, fmt.Errorf("failed to json decode credential payload: %w", err)
	}
	return vcp, nil
}

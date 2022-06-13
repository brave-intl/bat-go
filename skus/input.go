package skus

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/logging"
)

// VerifyCredentialRequestV1 includes an opaque subscription credential blob
type VerifyCredentialRequestV1 struct {
	Type         string  `json:"type" valid:"in(single-use|time-limited|time-limited-v2)"`
	Version      float64 `json:"version" valid:"-"`
	SKU          string  `json:"sku" valid:"-"`
	MerchantID   string  `json:"merchantId" valid:"-"`
	Presentation string  `json:"presentation" valid:"base64"`
}

//GetSku - implement credential interface
func (vcr *VerifyCredentialRequestV1) GetSku(ctx context.Context) string {
	return vcr.SKU
}

//GetType - implement credential interface
func (vcr *VerifyCredentialRequestV1) GetType(ctx context.Context) string {
	return vcr.Type
}

//GetMerchantID - implement credential interface
func (vcr *VerifyCredentialRequestV1) GetMerchantID(ctx context.Context) string {
	return vcr.MerchantID
}

//GetPresentation - implement credential interface
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

//GetSku - implement credential interface
func (vcr *VerifyCredentialRequestV2) GetSku(ctx context.Context) string {
	return vcr.SKU
}

//GetType - implement credential interface
func (vcr *VerifyCredentialRequestV2) GetType(ctx context.Context) string {
	if vcr.CredentialOpaque == nil {
		return ""
	}
	return vcr.CredentialOpaque.Type
}

//GetMerchantID - implement credential interface
func (vcr *VerifyCredentialRequestV2) GetMerchantID(ctx context.Context) string {
	return vcr.MerchantID
}

//GetPresentation - implement credential interface
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

const (
	appleVendor  Vendor = "ios"
	googleVendor        = "android"
)

var errInvalidVendor = errors.New("invalid vendor")

// Vendor vendor url input param
type Vendor string

// String - stringer implementation
func (v *Vendor) String() string {
	return string(*v)
}

// Validate - take raw []byte input and populate id with the ID
func (v *Vendor) Validate(ctx context.Context) error {
	if *v != appleVendor && *v != googleVendor {
		return fmt.Errorf("%s is not a valid vendor: %w", v, errInvalidVendor)
	}
	return nil
}

// Decode - take raw []byte input and populate id with the ID
func (v *Vendor) Decode(ctx context.Context, input []byte) error {
	if len(input) == 0 {
		return inputs.ErrIDDecodeEmpty
	}
	*v = Vendor(string(input))
	return nil
}

// SubmitReceiptRequestV1 - receipt submission request
type SubmitReceiptRequestV1 struct {
	Type           Vendor `json:"type" valid:"in(ios,android)"`
	Blob           string `json:"raw_receipt" valid:"required"`
	Package        string `json:"package"`
	SubscriptionID string `json:"subscription_id"`
}

// Decode - take raw input and populate the struct
func (srrv1 *SubmitReceiptRequestV1) Decode(ctx context.Context, input []byte) error {
	// base64 decode the bytes
	buf := []byte{}
	if _, err := base64.StdEncoding.Decode(buf, input); err != nil {
		return fmt.Errorf("failed to decode input base64: %w", err)
	}
	// read the json values
	if err := json.Unmarshal(buf, srrv1); err != nil {
		return fmt.Errorf("failed to decode input json: %w", err)
	}
	return nil
}

// Validate - validate the struct
func (srrv1 *SubmitReceiptRequestV1) Validate(ctx context.Context) error {
	// validate struct
	if _, err := govalidator.ValidateStruct(srrv1); err != nil {
		return fmt.Errorf("failed to validate structure: %w", err)
	}
	if err := srrv1.Type.Validate(ctx); err != nil {
		return fmt.Errorf("failed to validate vendor: %w", err)
	}
	return nil
}

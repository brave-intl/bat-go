package wallet

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"gopkg.in/square/go-jose.v2/jwt"
)

var (
	// ErrMissingSignedCreationRequest - required parameter missing from request
	ErrMissingSignedCreationRequest = errors.New("missing signed creation request")
	// ErrMissingSignedLinkingRequest - required parameter missing from request
	ErrMissingSignedLinkingRequest = errors.New("missing signed linking request")
	// ErrInvalidJSON - the input json is invalid
	ErrInvalidJSON = errors.New("invalid json")
	// ErrMissingLinkingInfo - required parameter missing from request
	ErrMissingLinkingInfo = errors.New("missing linking information")
)

// UpholdCreationRequest - the structure for a brave provider wallet creation request
type UpholdCreationRequest struct {
	SignedCreationRequest string `json:"signedCreationRequest"`
	PublicKey             string `json:"-"`
}

// Validate - implementation of validatable interface
func (ucr *UpholdCreationRequest) Validate(ctx context.Context) error {
	// validate there is a signed creation request
	if ucr.SignedCreationRequest == "" {
		return ErrMissingSignedCreationRequest
	}
	return nil
}

// Decode - implementation of  decodable interface
func (ucr *UpholdCreationRequest) Decode(ctx context.Context, v []byte) error {
	if err := inputs.DecodeJSON(ctx, v, ucr); err != nil {
		return fmt.Errorf("failed to decode json: %w", err)
	}
	// extract public key from the base64 encoded signing request headers

	b, err := base64.StdEncoding.DecodeString(ucr.SignedCreationRequest)
	if err != nil {
		return fmt.Errorf("failed to decode signed creation request: %w", err)
	}

	var signedTx uphold.HTTPSignedRequest
	err = json.Unmarshal(b, &signedTx)
	if err != nil {
		return fmt.Errorf("failed to decode signed creation request: %w", err)
	}

	_, err = govalidator.ValidateStruct(signedTx)
	if err != nil {
		return fmt.Errorf("failed to decode signed creation request: %w", err)
	}

	var body map[string]interface{}
	err = json.Unmarshal([]byte(signedTx.Body), &body)
	if err != nil {
		return fmt.Errorf("failed to decode signed creation request: %w", err)
	}

	pk, exists := body["publicKey"]
	if !exists {
		return errors.New("failed to decode signed creation request: no publicKey in body")
	}

	publicKey, ok := pk.(string)
	if !ok {
		return errors.New("failed to decode signed creation request: bad publicKey in body")
	}

	// put public key from request in ucr.PublicKey
	ucr.PublicKey = publicKey

	return nil
}

// HandleErrors - handle any errors from this request
func (ucr *UpholdCreationRequest) HandleErrors(err error) *handlers.AppError {
	issues := map[string]string{}
	if errors.Is(err, ErrInvalidJSON) {
		issues["invalidJSON"] = err.Error()
	}

	var merr *errorutils.MultiError
	if errors.As(err, &merr) {
		for _, e := range merr.Errs {
			if strings.Contains(e.Error(), "failed decoding") {
				issues["decoding"] = e.Error()
			}
			if strings.Contains(e.Error(), "failed validation") {
				issues["validation"] = e.Error()
			}
			if errors.Is(e, ErrMissingSignedCreationRequest) {
				issues["signedCreationRequest"] = "value is required"
			}
		}
	}
	return handlers.ValidationError("uphold create wallet request validation errors", issues)
}

// BraveCreationRequest - the structure for a brave provider wallet creation request
type BraveCreationRequest struct{}

// Validate - implementation of validatable interface
func (bcr *BraveCreationRequest) Validate(ctx context.Context) error {
	return nil
}

// Decode - implementation of  decodable interface
func (bcr *BraveCreationRequest) Decode(ctx context.Context, v []byte) error {
	return nil
}

// HandleErrors - handle any errors from this request
func (bcr *BraveCreationRequest) HandleErrors(err error) *handlers.AppError {
	issues := map[string]string{}
	if errors.Is(err, ErrInvalidJSON) {
		issues["invalidJSON"] = err.Error()
	}

	var merr *errorutils.MultiError
	if errors.As(err, &merr) {
		for _, e := range merr.Errs {
			if strings.Contains(e.Error(), "failed decoding") {
				issues["decoding"] = e.Error()
			}
			if strings.Contains(e.Error(), "failed validation") {
				issues["validation"] = e.Error()
			}
		}
	}
	return handlers.ValidationError("brave create wallet request validation errors", issues)
}

// LinkUpholdDepositAccountRequest - the structure for a linking request for uphold deposit account
type LinkUpholdDepositAccountRequest struct {
	SignedLinkingRequest string `json:"signedLinkingRequest"`
	AnonymousAddress     string `json:"anonymousAddress"`
}

// Validate - implementation of validatable interface
func (ludar *LinkUpholdDepositAccountRequest) Validate(ctx context.Context) error {
	var merr = new(errorutils.MultiError)
	if ludar.SignedLinkingRequest == "" {
		merr.Append(errors.New("failed to validate 'signedLinkingRequest': must not be empty"))
	}
	if ludar.AnonymousAddress != "" && !govalidator.IsUUID(ludar.AnonymousAddress) {
		merr.Append(errors.New("failed to validate 'anonymousAddress': must be uuid"))
	}
	if merr.Count() > 0 {
		return merr
	}
	return nil
}

// Decode - implementation of  decodable interface
func (ludar *LinkUpholdDepositAccountRequest) Decode(ctx context.Context, v []byte) error {
	if err := inputs.DecodeJSON(ctx, v, ludar); err != nil {
		return fmt.Errorf("failed to decode json: %w", err)
	}
	return nil
}

// HandleErrors - handle any errors from this request
func (ludar *LinkUpholdDepositAccountRequest) HandleErrors(err error) *handlers.AppError {
	issues := map[string]string{}
	if errors.Is(err, ErrInvalidJSON) {
		issues["invalidJSON"] = err.Error()
	}

	var merr *errorutils.MultiError
	if errors.As(err, &merr) {
		for _, e := range merr.Errs {
			if strings.Contains(e.Error(), "failed decoding") {
				issues["decoding"] = e.Error()
			}
			if strings.Contains(e.Error(), "failed validation") {
				issues["validation"] = e.Error()
			}
		}
	}
	return handlers.ValidationError("brave create wallet request validation errors", issues)
}

// LinkBraveDepositAccountRequest - the structure for a linking request for uphold deposit account
type LinkBraveDepositAccountRequest struct {
	DepositDestination string `json:"depositDestination"`
}

// Validate - implementation of validatable interface
func (lbdar *LinkBraveDepositAccountRequest) Validate(ctx context.Context) error {
	var merr = new(errorutils.MultiError)
	if lbdar.DepositDestination != "" && !govalidator.IsUUID(lbdar.DepositDestination) {
		merr.Append(errors.New("failed to validate 'depositDestination': must be uuid"))
	}
	if merr.Count() > 0 {
		return merr
	}
	return nil
}

// Decode - implementation of  decodable interface
func (lbdar *LinkBraveDepositAccountRequest) Decode(ctx context.Context, v []byte) error {
	if err := inputs.DecodeJSON(ctx, v, lbdar); err != nil {
		return fmt.Errorf("failed to decode json: %w", err)
	}
	return nil
}

// HandleErrors - handle any errors from this request
func (lbdar *LinkBraveDepositAccountRequest) HandleErrors(err error) *handlers.AppError {
	issues := map[string]string{}
	if errors.Is(err, ErrInvalidJSON) {
		issues["invalidJSON"] = err.Error()
	}

	var merr *errorutils.MultiError
	if errors.As(err, &merr) {
		for _, e := range merr.Errs {
			if strings.Contains(e.Error(), "failed decoding") {
				issues["decoding"] = e.Error()
			}
			if strings.Contains(e.Error(), "failed validation") {
				issues["validation"] = e.Error()
			}
		}
	}
	return handlers.ValidationError("brave link wallet request validation errors", issues)
}

// BitFlyerLinkingRequest - the structure for a brave provider wallet creation request
type BitFlyerLinkingRequest struct {
	LinkingInfo string `json:"linkingInfo"`

	DepositID   string `json:"-"`
	AccountHash string `json:"-"`
}

// BitFlyerLinkingInfo - jwt structure of the linking info
type BitFlyerLinkingInfo struct {
	DepositID         string    `json:"deposit_id"`
	RequestID         string    `json:"request_id"`
	AccountHash       string    `json:"account_hash"`
	ExternalAccountID string    `json:"external_account_id"`
	Timestamp         time.Time `json:"timestamp"`
}

// Validate - implementation of validatable interface
func (blr *BitFlyerLinkingRequest) Validate(ctx context.Context) error {
	// validate there is a signed creation request
	if blr.LinkingInfo == "" {
		return ErrMissingSignedLinkingRequest
	}

	// get the bitflyer jwt key from ctx
	jwtKey, err := appctx.GetByteSliceFromContext(ctx, appctx.BitFlyerJWTKeyCTXKey)
	if err != nil {
		return fmt.Errorf("configuration error, no jwt validation key: %w", err)
	}

	tok, err := jwt.ParseSigned(blr.LinkingInfo)
	if err != nil {
		return fmt.Errorf("failed to parse the linking info jwt token: %w", err)
	}

	base := jwt.Claims{}
	linkingInfo := BitFlyerLinkingInfo{}

	if err := tok.Claims(jwtKey, &base, &linkingInfo); err != nil {
		return fmt.Errorf("failed to parse the linking info jwt token: %w", err)
	}

	blr.DepositID = linkingInfo.DepositID
	blr.AccountHash = linkingInfo.AccountHash

	if blr.AccountHash == "" || blr.DepositID == "" {
		// failed to extract claims, or the token is invalid
		return fmt.Errorf("failed to parse claims: %w", err)
	}

	return nil
}

// Decode - implementation of  decodable interface
func (blr *BitFlyerLinkingRequest) Decode(ctx context.Context, v []byte) error {
	if err := inputs.DecodeJSON(ctx, v, blr); err != nil {
		return fmt.Errorf("failed to decode json: %w", err)
	}

	// TODO: pull out the DepositID and AccountHash from the JWT
	// and set them in blr
	return nil
}

// HandleErrors - handle any errors from this request
func (blr *BitFlyerLinkingRequest) HandleErrors(err error) *handlers.AppError {
	issues := map[string]string{}
	if errors.Is(err, ErrInvalidJSON) {
		issues["invalidJSON"] = err.Error()
	}

	var merr *errorutils.MultiError
	if errors.As(err, &merr) {
		for _, e := range merr.Errs {
			if strings.Contains(e.Error(), "failed decoding") {
				issues["decoding"] = e.Error()
			}
			if strings.Contains(e.Error(), "failed validation") {
				issues["validation"] = e.Error()
			}
			if errors.Is(e, ErrMissingLinkingInfo) {
				issues["linkingInfo"] = "value is required"
			}
		}
	}
	return handlers.ValidationError("bitflyer deposit wallet linking request validation errors", issues)
}

package wallet

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/asaskevich/govalidator"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
)

var (
	// ErrMissingSignedCreationRequest - required parameter missing from request
	ErrMissingSignedCreationRequest = errors.New("missing signed creation request")
	// ErrInvalidJSON - the input json is invalid
	ErrInvalidJSON = errors.New("invalid json")
)

// UpholdCreationRequest - the structure for a brave provider wallet creation request
type UpholdCreationRequest struct {
	SignedCreationRequest string `json:"signedCreationRequest"`
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

// ClaimUpholdWalletRequest - the structure for a brave provider wallet creation request
type ClaimUpholdWalletRequest struct {
	SignedCreationRequest string `json:"signedCreationRequest"`
	AnonymousAddress      string `json:"anonymousAddress"`
}

// Validate - implementation of validatable interface
func (cuw *ClaimUpholdWalletRequest) Validate(ctx context.Context) error {
	var merr = new(errorutils.MultiError)
	if cuw.SignedCreationRequest == "" {
		merr.Append(errors.New("failed to validate 'signedCreationRequest': must not be empty"))
	}
	if cuw.AnonymousAddress != "" && !govalidator.IsUUID(cuw.AnonymousAddress) {
		merr.Append(errors.New("failed to validate 'anonymousAddress': must be uuid"))
	}
	if merr.Count() > 0 {
		return merr
	}
	return nil
}

// Decode - implementation of  decodable interface
func (cuw *ClaimUpholdWalletRequest) Decode(ctx context.Context, v []byte) error {
	if err := inputs.DecodeJSON(ctx, v, cuw); err != nil {
		return fmt.Errorf("failed to decode json: %w", err)
	}
	return nil
}

// HandleErrors - handle any errors from this request
func (cuw *ClaimUpholdWalletRequest) HandleErrors(err error) *handlers.AppError {
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

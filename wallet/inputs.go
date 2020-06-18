package wallet

import (
	"context"
	"errors"
	"fmt"
	"strings"

	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
)

var (
	ErrMissingSignedCreationRequest = errors.New("missing signed creation request")
	ErrInvalidJSON                  = errors.New("invalid json")
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

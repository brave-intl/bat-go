package uphold

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DrainData - uphold specific drain error "data" wrapper for errorutils
type DrainData struct {
	code string
}

// NewDrainData - get uphold specific drain data from the coded error
func NewDrainData(c Coded) *DrainData {
	return &DrainData{
		code: strings.ToLower(c.GetCode()),
	}
}

// DrainCode - implement the drain code rendering of the error
func (dd *DrainData) DrainCode() (string, bool) {
	return fmt.Sprintf("uphold_%s", dd.code), true
}

// Coded - interface for things that have codes, such as errors
type Coded interface {
	GetCode() string
}

type upholdBaseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// TODO just use json.RawMessage

type upholdDenominationValidationErrors struct {
	AmountError []upholdBaseError `json:"amount,omitempty"`
	Data        json.RawMessage   `json:",omitempty"`
}

type upholdDenominationErrors struct {
	Code             string                             `json:"code"`
	ValidationErrors upholdDenominationValidationErrors `json:"errors,omitempty"`
	Data             json.RawMessage                    `json:",omitempty"`
}

type upholdValidationErrors struct {
	SignatureError     []upholdBaseError        `json:"signature,omitempty"`
	DenominationErrors upholdDenominationErrors `json:"denomination,omitempty"`
	DestinationErrors  []upholdBaseError        `json:"destination,omitempty"`
	Data               json.RawMessage          `json:",omitempty"`
}

type upholdError struct {
	Message          string                 `json:"error,omitempty"`
	Code             string                 `json:"code"`
	ValidationErrors upholdValidationErrors `json:"errors,omitempty"`
	Data             json.RawMessage        `json:",omitempty"`
}

// Code - implement coded interface
func (uhErr upholdError) GetCode() string {
	return uhErr.Code
}

func (uhErr upholdError) NotFoundError() bool {
	return uhErr.Code == "not_found"
}

func (uhErr upholdError) ValidationError() bool {
	return uhErr.Code == "validation_failed"
}

func (uhErr upholdError) AlreadyExistsError() bool {
	return uhErr.Code == "transaction_already_exists"
}

func (uhErr upholdError) DenominationError() bool {
	return uhErr.ValidationError() && uhErr.ValidationErrors.DenominationErrors.Code == "validation_failed"
}

func (uhErr upholdError) DestinationError() bool {
	return uhErr.ValidationError() && len(uhErr.ValidationErrors.DestinationErrors) > 0
}

func (uhErr upholdError) InvalidDestination() bool {
	return uhErr.DestinationError()
}

func (uhErr upholdError) AmountError() bool {
	return uhErr.DenominationError() && len(uhErr.ValidationErrors.DenominationErrors.ValidationErrors.AmountError) > 0
}

func (uhErr upholdError) InsufficientBalance() bool {
	if uhErr.AmountError() {
		for _, ae := range uhErr.ValidationErrors.DenominationErrors.ValidationErrors.AmountError {
			if ae.Code == "sufficient_funds" {
				return true
			}
		}
	}
	return false
}

func (uhErr upholdError) ForbiddenError() bool {
	return uhErr.Code == "forbidden"
}

func (uhErr upholdError) InvalidSignature() bool {
	return uhErr.ValidationError() && len(uhErr.ValidationErrors.SignatureError) > 0
}

func (uhErr upholdError) String() string {
	if uhErr.InsufficientBalance() {
		for _, ae := range uhErr.ValidationErrors.DenominationErrors.ValidationErrors.AmountError {
			if ae.Code == "sufficient_funds" {
				return ae.Message
			}
		}
	} else if uhErr.InvalidSignature() {
		return "Signature: " + uhErr.ValidationErrors.SignatureError[0].Message
	}
	b, err := json.Marshal(&uhErr)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func (uhErr upholdError) Error() string {
	return "UpholdError: " + uhErr.String()
}

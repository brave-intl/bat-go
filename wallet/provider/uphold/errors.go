package uphold

import (
	"encoding/json"
)

type upholdBaseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// TODO just use json.RawMessage

type upholdDenominationValidationErrors struct {
	AmountError []upholdBaseError `json:"amount, omitempty"`
	Data        json.RawMessage   `json:",omitempty"`
}

type upholdDenominationErrors struct {
	Code                               string `json:"code"`
	upholdDenominationValidationErrors `json:"errors,omitempty"`
	Data                               json.RawMessage `json:",omitempty"`
}

type upholdValidationErrors struct {
	SignatureError     []upholdBaseError        `json:"signature, omitempty"`
	DenominationErrors upholdDenominationErrors `json:"denomination, omitempty"`
	Data               json.RawMessage          `json:",omitempty"`
}

type upholdError struct {
	Message          string                 `json:"error,omitempty"`
	Code             string                 `json:"code"`
	ValidationErrors upholdValidationErrors `json:"errors,omitempty"`
	Data             json.RawMessage        `json:",omitempty"`
}

func (err upholdError) ValidationError() bool {
	return err.Code == "validation_failed"
}

func (err upholdError) DenominationError() bool {
	return err.ValidationError() && err.ValidationErrors.DenominationErrors.Code == "validation_failed"
}

func (err upholdError) AmountError() bool {
	return err.DenominationError() && len(err.ValidationErrors.DenominationErrors.AmountError) > 0
}

func (err upholdError) InsufficientBalance() bool {
	if err.AmountError() {
		for _, ae := range err.ValidationErrors.DenominationErrors.AmountError {
			if ae.Code == "sufficient_funds" {
				return true
			}
		}
	}
	return false
}

func (err upholdError) InvalidSignature() bool {
	return err.ValidationError() && len(err.ValidationErrors.SignatureError) > 0
}

func (err upholdError) String() string {
	if err.InsufficientBalance() {
		for _, ae := range err.ValidationErrors.DenominationErrors.AmountError {
			if ae.Code == "sufficient_funds" {
				return ae.Message
			}
		}
	} else if err.InvalidSignature() {
		return "Signature: " + err.ValidationErrors.SignatureError[0].Message
	}
	b, _ := json.Marshal(&err)
	return string(b)
}

func (err upholdError) Error() string {
	return "UpholdError: " + err.String()
}

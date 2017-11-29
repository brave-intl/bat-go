package uphold

import (
	"encoding/json"
)

type UpholdBaseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// TODO just use json.RawMessage

type UpholdDenominationValidationErrors struct {
	AmountError []UpholdBaseError `json:"amount, omitempty"`
	Data        json.RawMessage   `json:",omitempty"`
}

type UpholdDenominationErrors struct {
	Code                               string `json:"code"`
	UpholdDenominationValidationErrors `json:"errors,omitempty"`
	Data                               json.RawMessage `json:",omitempty"`
}

type UpholdValidationErrors struct {
	SignatureError     []UpholdBaseError        `json:"signature, omitempty"`
	DenominationErrors UpholdDenominationErrors `json:"denomination, omitempty"`
	Data               json.RawMessage          `json:",omitempty"`
}

type UpholdError struct {
	Message          string                 `json:"error,omitempty"`
	Code             string                 `json:"code"`
	ValidationErrors UpholdValidationErrors `json:"errors,omitempty"`
	Data             json.RawMessage        `json:",omitempty"`
}

type Fuck struct {
	Code string `json:"code"`
}

func (err UpholdError) ValidationError() bool {
	if err.Code == "validation_failed" {
		return true
	}
	return false
}

func (err UpholdError) DenominationError() bool {
	if err.ValidationError() {
		if err.ValidationErrors.DenominationErrors.Code == "validation_failed" {
			return true
		}
	}
	return false
}

func (err UpholdError) AmountError() bool {
	if err.DenominationError() {
		if len(err.ValidationErrors.DenominationErrors.AmountError) > 0 {
			return true
		}
	}
	return false
}

func (err UpholdError) InsufficientBalance() bool {
	if err.AmountError() {
		for _, ae := range err.ValidationErrors.DenominationErrors.AmountError {
			if ae.Code == "sufficient_funds" {
				return true
			}
		}
	}
	return false
}

func (err UpholdError) InvalidSignature() bool {
	if err.ValidationError() {
		if len(err.ValidationErrors.SignatureError) > 0 {
			return true
		}
	}
	return false
}

func (err UpholdError) String() string {
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

func (err UpholdError) Error() string {
	return "UpholdError: " + err.String()
}

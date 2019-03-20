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
	Data               json.RawMessage          `json:",omitempty"`
}

type upholdError struct {
	Message          string                 `json:"error,omitempty"`
	Code             string                 `json:"code"`
	ValidationErrors upholdValidationErrors `json:"errors,omitempty"`
	Data             json.RawMessage        `json:",omitempty"`
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

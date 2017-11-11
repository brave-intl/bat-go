package uphold

import (
	"encoding/json"
	"testing"
)

func TestInsufficientBalance(t *testing.T) {
	errJson := []byte(`{"code":"validation_failed","errors":{"denomination":{"code":"validation_failed","errors":{"amount":[{"code":"sufficient_funds","message":"Not enough funds for the specified amount"}]}}}}`)
	var uhErr UpholdError
	err := json.Unmarshal(errJson, &uhErr)
	if err != nil {
		t.Error("Unexpected error during uphold error unmarshal")
	}

	if !uhErr.InsufficientBalance() {
		t.Error("Expected resulting error to be for insufficient balance")
	}
	if uhErr.InvalidSignature() {
		t.Error("Expected resulting error to only be for insufficient balance")
	}
	if uhErr.Error() != "UpholdError: Not enough funds for the specified amount" {
		t.Error("Incorrect resulting error string")
	}
}

func TestInvalidSignature(t *testing.T) {
	errJson := []byte(`{"code":"validation_failed","errors":{"signature":[{"code":"required","message":"This value is required"}]}}`)
	var uhErr UpholdError
	err := json.Unmarshal(errJson, &uhErr)
	if err != nil {
		t.Error("Unexpected error during uphold error unmarshal")
	}

	if !uhErr.InvalidSignature() {
		t.Error("Expected resulting error to be for invalid signature")
	}
	if uhErr.InsufficientBalance() {
		t.Error("Expected resulting error to only be for invalid signature")
	}
	if uhErr.Error() != "UpholdError: Signature: This value is required" {
		t.Error("Incorrect resulting error string")
	}

	errJson = []byte(`{"code":"validation_failed","errors":{"signature":[{"code":"invalid","message":"This value is not valid"}]}}`)
	uhErr = UpholdError{}
	err = json.Unmarshal(errJson, &uhErr)
	if err != nil {
		t.Error("Unexpected error during uphold error unmarshal")
	}

	if !uhErr.InvalidSignature() {
		t.Error("Expected resulting error to be for invalid signature")
	}
	if uhErr.InsufficientBalance() {
		t.Error("Expected resulting error to only be for invalid signature")
	}
	if uhErr.Error() != "UpholdError: Signature: This value is not valid" {
		t.Error("Incorrect resulting error string")
	}
}

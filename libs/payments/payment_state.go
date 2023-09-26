package payments

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// PaymentState defines data for qldbPaymentTransitionHistoryEntry.
type PaymentState struct {
	// Serialized AuthenticatedPaymentState. Should only ever be access via GetSafePaymentState,
	// which does all of the needed validation of the state
	UnsafePaymentState []byte     `ion:"data"`
	Signature          []byte     `ion:"signature"`
	ID                 uuid.UUID `ion:"idempotencyKey"`
}

// ToStructuredUnsafePaymentState only unmarshals an ToStructuredUnsafePaymentState from the
// UnsafePaymentState field in a PaymentState. It does NOT do the requisite validation and should
// not be used except to get the fields needed to do that validation.
func (p *PaymentState) ToStructuredUnsafePaymentState() (*AuthenticatedPaymentState, error) {
	var unauthenticatedState AuthenticatedPaymentState
	err := json.Unmarshal(p.UnsafePaymentState, &unauthenticatedState)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal record data for conversion from qldbPaymentTransitionHistoryEntry to Transaction: %w",
			err,
		)
	}
	return &unauthenticatedState, nil
}

// GenerateIdempotencyKey returns a UUID v5 ID if the ID on the Transaction matches its expected
// value. Otherwise, it returns an error.
func (p *PaymentState) GenerateIdempotencyKey() (uuid.UUID, error) {
	authenticatedState, err := p.ToStructuredUnsafePaymentState()
	if err != nil {
		return uuid.Nil, err
	}
	generatedIdempotencyKey := authenticatedState.GenerateIdempotencyKey()
	if generatedIdempotencyKey != p.ID {
		return uuid.Nil, fmt.Errorf(
			"ID does not match transaction fields: have %s, want %s",
			p.ID,
			generatedIdempotencyKey,
		)
	}
	return p.ID, nil
}

// SetIdempotencyKey assigns a UUID v5 value to PaymentState.ID.
func (p *PaymentState) SetIdempotencyKey() error {
	authenticatedPaymentState, err := p.ToStructuredUnsafePaymentState()
	if err != nil {
		return err
	}
	generatedIdempotencyKey := authenticatedPaymentState.GenerateIdempotencyKey()
	p.ID = generatedIdempotencyKey
	return nil
}

func UnsignedPaymentStateFromDetails(details PaymentDetails) (*PaymentState, error) {
	authenticatedState := AuthenticatedPaymentState{
		PaymentDetails: details,
		Status: Prepared,
	}
	authenticatedStateString, err := json.Marshal(authenticatedState)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal authenticated state: %w", err)
	}
	return &PaymentState{
		UnsafePaymentState: authenticatedStateString,
		ID: authenticatedState.GenerateIdempotencyKey(),
	}, nil
}

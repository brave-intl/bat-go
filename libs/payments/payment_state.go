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

func (p *PaymentState) ToAuthenticatedPaymentState() (*AuthenticatedPaymentState, error) {
	var txn AuthenticatedPaymentState
	err := json.Unmarshal(p.UnsafePaymentState, &txn)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal record data for conversion from qldbPaymentTransitionHistoryEntry to Transaction: %w",
			err,
		)
	}
	return &txn, nil
}

// GenerateIdempotencyKey returns a UUID v5 ID if the ID on the Transaction matches its expected
// value. Otherwise, it returns an error.
func (p *PaymentState) GenerateIdempotencyKey(namespace uuid.UUID) (uuid.UUID, error) {
	authenticatedState, err := p.ToAuthenticatedPaymentState()
	if err != nil {
		return uuid.New(), err
	}
	generatedIdempotencyKey := authenticatedState.GenerateIdempotencyKey(namespace)
	if generatedIdempotencyKey != p.ID {
		return uuid.New(), fmt.Errorf(
			"ID does not match transaction fields: have %s, want %s",
			p.ID,
			generatedIdempotencyKey,
		)
	}
	return p.ID, nil
}

// SetIdempotencyKey assigns a UUID v5 value to PaymentState.ID.
func (p *PaymentState) SetIdempotencyKey(namespace uuid.UUID) error {
	authenticatedPaymentState, err := p.ToAuthenticatedPaymentState()
	if err != nil {
		return err
	}
	generatedIdempotencyKey := authenticatedPaymentState.GenerateIdempotencyKey(namespace)
	p.ID = generatedIdempotencyKey
	return nil
}

func PaymentStateFromDetails(details PaymentDetails, namespace uuid.UUID) (*PaymentState, error) {
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
		ID: authenticatedState.GenerateIdempotencyKey(namespace),
	}, nil
}

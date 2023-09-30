package payments

import (
	"crypto"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// PaymentState is the high level structure which is stored in a datastore.
// It includes the full payment state as well as authentication information which proves it is a valid
// object authored by the enclave. Accessing the payment state directly is considered unsafe, one must
// go through a getter which verifies the history.
type PaymentState struct {
	// Serialized AuthenticatedPaymentState. Should only ever be access via GetSafePaymentState,
	// which does all of the needed validation of the state
	UnsafePaymentState []byte    `ion:"data"`
	Signature          []byte    `ion:"signature"`
	PublicKey          []byte    `ion:"publicKey"`
	ID                 uuid.UUID `ion:"idempotencyKey"`
}

// Verifier is an interface for verifying signatures
type Verifier interface {
	Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error)
}

// PaymentStateHistory is a sequence of payment states.
type PaymentStateHistory []PaymentState

// GetAuthenticatedPaymentState by performing the appropriate validation.
func (p PaymentStateHistory) GetAuthenticatedPaymentState(verifier Verifier, documentID string) (*AuthenticatedPaymentState, error) {
	// iterate through our payment states checking:
	// 1. the signature
	// 2. the documentID matches any internal documentIDs
	// 3. the transition was valid
	var authenticatedState AuthenticatedPaymentState
	for i, state := range []PaymentState(p) {
		var unsafeState AuthenticatedPaymentState
		valid, err := verifier.Verify(state.UnsafePaymentState, state.Signature, crypto.Hash(0))
		if err != nil {
			return nil, fmt.Errorf("signature validation for state with document ID %s failed: %w", documentID, err)
		}
		if !valid {
			return nil, fmt.Errorf("signature for state with document ID %s was not valid", documentID)
		}

		err = json.Unmarshal(state.UnsafePaymentState, &unsafeState)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal transaction data: %w", err)
		}

		if unsafeState.DocumentID != "" && unsafeState.DocumentID != documentID {
			return nil, fmt.Errorf("internal document ID %s did not match expected document ID %s", unsafeState.DocumentID, documentID)
		}
		for _, authorization := range unsafeState.Authorizations {
			if authorization.DocumentID != documentID {
				return nil, fmt.Errorf("internal authorization document ID %s did not match expected document ID %s", authorization.DocumentID, documentID)
			}
		}

		if i == 0 {
			// must always start in prepared
			if unsafeState.Status != Prepared {
				return nil, &InvalidTransitionState{}
			}
		} else {
			// New state should be present in the list of valid next states for the
			// "previous" (current) state.
			if !authenticatedState.NextStateValid(unsafeState.Status) {
				return nil, &InvalidTransitionState{
					From: string(authenticatedState.Status),
					To:   string(unsafeState.Status),
				}
			}
		}

		authenticatedState = unsafeState
	}

	authenticatedState.DocumentID = documentID
	return &authenticatedState, nil
}

package payments

import (
	"crypto"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PaymentDetails captures the key details of a particular payment to be executed as part of a settlement.
// This is the minimum information needed to execute a transfer.
type PaymentDetails struct {
	Amount    decimal.Decimal `json:"amount" valid:"required"`
	To        string          `json:"to" valid:"required"`
	From      string          `json:"from" valid:"required"`
	Custodian string          `json:"custodian" valid:"in(uphold|gemini|bitflyer)"`
	PayoutID  string          `json:"payoutId" valid:"required"`
	Currency  string          `json:"currency" valid:"required"`
}

// PaymentAuthorization represents a single authorization from a payment authorizer indicating that
// the payout represented by a document ID should be processed
type PaymentAuthorization struct {
	KeyID      string `json:"keyId" valid:"required"`
	DocumentID string `json:"documentId" valid:"required"`
}

// AuthenticatedPaymentState is a payment state whose providence has been authenticated as originating
// within an enclave.
type AuthenticatedPaymentState struct {
	PaymentDetails
	Status         PaymentStatus          `json:"status"`
	Authorizations []PaymentAuthorization `json:"authorizations"`
	DryRun         *string                `json:"dryRun"`
	LastError      *PaymentError          `json:"lastError"`
	DocumentID     string                 `json:"documentID"`
}

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

// PaymentStateHistory is a sequence of payment states.
type PaymentStateHistory []PaymentState

// ToPaymentState creates an unsigned PaymentState from PaymentDetails
func (p PaymentDetails) ToPaymentState(dryRun *string) (*PaymentState, error) {
	idempotencyNamespace := uuid.MustParse("3c0e75eb-9150-40b4-a988-a017d115de3c")
	id := uuid.NewSHA1(
		idempotencyNamespace,
		[]byte(fmt.Sprintf(
			"%s%s%s%s%s%s",
			p.To,
			p.From,
			p.Currency,
			p.Amount,
			p.Custodian,
			p.PayoutID,
		)),
	)

	authenticatedState := AuthenticatedPaymentState{
		PaymentDetails: p,
		Status:         Prepared,
		Authorizations: []PaymentAuthorization{},
		DryRun:         dryRun,
		LastError:      nil,
		DocumentID:     "",
	}
	bytes, err := json.Marshal(authenticatedState)
	if err != nil {
		return nil, err
	}

	state := PaymentState{
		UnsafePaymentState: bytes,
		Signature:          []byte{},
		PublicKey:          []byte{},
		ID:                 id,
	}

	return &state, nil
}

// NextStateValid returns true if nextState is a valid transition from the current one
func (t *AuthenticatedPaymentState) NextStateValid(nextState PaymentStatus) bool {
	if t.Status == nextState {
		return true
	}
	// New transaction state should be present in the list of valid next states for the current
	// state.
	return statusListContainsStatus(t.Status.GetValidTransitions(), nextState)
}

// shouldDryRun returns whether we should skip logic for the next state transition based on the dryRun flag
func (t *AuthenticatedPaymentState) shouldDryRun() bool {
	if t.DryRun == nil {
		return false
	}

	switch t.Status {
	case Prepared:
		return *t.DryRun == "prepare"
	case Authorized, Pending, Paid, Failed:
		return *t.DryRun == "submit"
	default:
		return false
	}
}

// statusListContainsStatus returns true if the status list contains the passed status
func statusListContainsStatus(s []PaymentStatus, e PaymentStatus) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

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

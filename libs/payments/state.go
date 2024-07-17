package payments

import (
	"context"
	"crypto"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PaymentDetails captures the key details of a particular payment to be executed as part of a settlement.
// This is the minimum information needed to execute a transfer.
type PaymentDetails struct {
	Amount    decimal.Decimal `json:"amount" valid:"required"`
	To        string          `json:"to" valid:"required"`
	From      string          `json:"from" valid:"required"`
	Custodian string          `json:"custodian" valid:"in(uphold|gemini|bitflyer|zebpay|solana)"`
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
	LastError      *PaymentError          `json:"lastError"`
	DocumentID     string                 `json:"documentID"`
	// ExternalIdempotency is for state machines which have third party idempotency values that are
	// not derministically generated from data that we control but that need to be retained between
	// calls to Pay(). For example, Solana requires the block hash and transaction signature to
	// guarantee idempotency.
	ExternalIdempotency []byte `json:"externalIdempotency"`
}

// PaymentState is the high level structure which is stored in a datastore.
// It includes the full payment state as well as authentication information which proves it is a valid
// object authored by the enclave. Accessing the payment state directly is considered unsafe, one must
// go through a getter which verifies the history.
type PaymentState struct {
	// Serialized AuthenticatedPaymentState. Should only ever be access via GetAuthenticatedPaymentState,
	// which does all of the needed validation of the state
	UnsafePaymentState []byte    `ion:"data"`
	Signature          []byte    `ion:"signature"`
	PublicKey          string    `ion:"publicKey"`
	ID                 uuid.UUID `ion:"idempotencyKey"`
	UpdatedAt          time.Time `ion:"-"`
}

// PaymentStateHistory is a sequence of payment states.
type PaymentStateHistory []PaymentState

// IdempotencyKey calculates the idempotency key for these PaymentDetails
func (p PaymentDetails) IdempotencyKey() uuid.UUID {
	return uuid.NewSHA1(
		uuid.MustParse("3c0e75eb-9150-40b4-a988-a017d115de3c"),
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
}

// ToAuthenticatedPaymentState creates an new AuthenticatedPaymentState from PaymentDetails
func (p PaymentDetails) ToAuthenticatedPaymentState() *AuthenticatedPaymentState {
	authenticatedState := AuthenticatedPaymentState{
		PaymentDetails: p,
	}
	return &authenticatedState
}

// ToPaymentState creates an unsigned PaymentState from an AuthenticatedPaymentState
func (t AuthenticatedPaymentState) ToPaymentState() (*PaymentState, error) {
	marshaledState, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}

	paymentState := PaymentState{
		UnsafePaymentState: marshaledState,
		ID:                 t.PaymentDetails.IdempotencyKey(),
	}

	return &paymentState, nil
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

// statusListContainsStatus returns true if the status list contains the passed status
func statusListContainsStatus(s []PaymentStatus, e PaymentStatus) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

// Sign this payment state, authenticating the contents of UnsafePaymentState
func (p *PaymentState) Sign(signer Signator, publicKey string) error {
	var err error
	p.Signature, err = signer.Sign(rand.Reader, p.UnsafePaymentState, crypto.Hash(0))
	if err != nil {
		return fmt.Errorf("Failed to sign payment state: %w", err)
	}
	p.PublicKey = publicKey
	return nil
}

// GetAuthenticatedPaymentState by performing the appropriate validation.
func (p PaymentStateHistory) GetAuthenticatedPaymentState(keystore Keystore, documentID string) (*AuthenticatedPaymentState, error) {
	// iterate through our payment states checking:
	// 1. the signature
	// 2. the documentID matches any internal documentIDs
	// 3. the transition was valid
	var authenticatedState AuthenticatedPaymentState
	for i, state := range []PaymentState(p) {
		_, verifier, err := keystore.LookupVerifier(context.Background(), state.PublicKey, state.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("signature validation for state with document ID %s failed: %w", documentID, err)
		}
		if verifier == nil {
			return nil, fmt.Errorf("failed to locate a verifier for the keyId %s", state.PublicKey)
		}

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

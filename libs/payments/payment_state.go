package payments

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"golang.org/x/exp/slices"

	"github.com/google/uuid"
)

// PaymentStatus is an integer representing transaction status.
type PaymentStatus string

// TxStateMachine describes types with the appropriate methods to be Driven as a state machine
const (
	// Prepared represents a record that has been prepared for authorization.
	Prepared PaymentStatus = "prepared"
	// Authorized represents a record that has been authorized.
	Authorized PaymentStatus = "authorized"
	// Pending represents a record that is being or has been submitted to a processor.
	Pending PaymentStatus = "pending"
	// Paid represents a record that has entered a finalized success state with a processor.
	Paid PaymentStatus = "paid"
	// Failed represents a record that has failed processing permanently.
	Failed PaymentStatus = "failed"
)

// Transitions represents the valid forward-transitions for each given state.
var Transitions = map[PaymentStatus][]PaymentStatus{
	Prepared:   {Authorized, Failed},
	Authorized: {Pending, Failed},
	Pending:    {Paid, Failed},
	Paid:       {},
	Failed:     {},
}

type PrepareRequest struct {
	PaymentDetails
	DryRun *string `json:"dryRun"` // determines dry-run
}

// PrepareResponse is sent to the client in response to a PrepareRequest. It must include
// an Attestation in the header of the response.
type PrepareResponse struct {
	PaymentDetails
	DocumentID string `json:"documentId,omitempty"`
}

type SubmitRequest struct {
	DocumentID string `json:"documentId,omitempty"`
}

type SubmitResponse struct {
	Status    PaymentStatus `json:"status" valid:"required"`
	LastError *PaymentError `json:"error,omitempty"`
}

type AuthenticatedPaymentState struct {
	PaymentDetails
	Status         PaymentStatus
	Authorizations []PaymentAuthorization `json:"authorizations"`
	DryRun         *string                `json:"dryRun"` // determines dry-run
	LastError      *PaymentError
	documentID     string
}

type PaymentDetails struct {
	Amount    decimal.Decimal `json:"amount" valid:"required"`
	To        string       `json:"to" valid:"required"`
	From      string       `json:"from" valid:"required"`
	Custodian string       `json:"custodian" valid:"in(uphold|gemini|bitflyer)"`
	PayoutID  string       `json:"payoutId" valid:"required"`
	Currency  string       `json:"currency" valid:"required"`
}

// PaymentAuthorization represents a single authorization from a payment authorizer indicating that
// the payout represented by a document ID should be processed
type PaymentAuthorization struct {
	KeyID      string `json:"keyId" valid:"required"`
	DocumentID string `json:"documentId" valid:"required"`
}

// qldbPaymentTransitionHistoryEntryBlockAddress defines blockAddress data for
// qldbPaymentTransitionHistoryEntry.
type qldbPaymentTransitionHistoryEntryBlockAddress struct {
	StrandID   string `ion:"strandId"`
	SequenceNo int64  `ion:"sequenceNo"`
}

// QLDBPaymentTransitionHistoryEntryHash defines hash for qldbPaymentTransitionHistoryEntry.
type QLDBPaymentTransitionHistoryEntryHash string

// qldbPaymentTransitionHistoryEntrySignature defines signature for
// qldbPaymentTransitionHistoryEntry.
type qldbPaymentTransitionHistoryEntrySignature []byte

// PaymentState defines data for qldbPaymentTransitionHistoryEntry.
type PaymentState struct {
	// Serialized AuthenticatedPaymentState. Should only ever be access via GetSafePaymentState,
	// which does all of the needed validation of the state
	UnsafePaymentState []byte     `ion:"data"`
	Signature          []byte     `ion:"signature"`
	ID                 *uuid.UUID `ion:"idempotencyKey"`
}

// qldbPaymentTransitionHistoryEntryMetadata defines metadata for qldbPaymentTransitionHistoryEntry
type qldbPaymentTransitionHistoryEntryMetadata struct {
	ID      string    `ion:"id"`
	TxID    string    `ion:"txId"`
	TxTime  time.Time `ion:"txTime"`
	Version int64     `ion:"version"`
}

// QLDBPaymentTransitionHistoryEntry defines top level entry for a QLDB transaction.
type QLDBPaymentTransitionHistoryEntry struct {
	BlockAddress qldbPaymentTransitionHistoryEntryBlockAddress `ion:"blockAddress"`
	Hash         QLDBPaymentTransitionHistoryEntryHash         `ion:"hash"`
	Data         PaymentState                                  `ion:"data"`
	Metadata     qldbPaymentTransitionHistoryEntryMetadata     `ion:"metadata"`
}

type qldbDocumentIDResult struct {
	documentID string `ion:"documentId"`
}

// GetValidTransitions returns valid transitions.
func (ts PaymentStatus) GetValidTransitions() []PaymentStatus {
	return Transitions[ts]
}

func (e *QLDBPaymentTransitionHistoryEntry) toTransaction() (*AuthenticatedPaymentState, error) {
	var txn AuthenticatedPaymentState
	err := json.Unmarshal(e.Data.UnsafePaymentState, &txn)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal record data for conversion from qldbPaymentTransitionHistoryEntry to Transaction: %w",
			err,
		)
	}
	return &txn, nil
}

func (p *PaymentState) toAuthenticatedPaymentState() (*AuthenticatedPaymentState, error) {
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

// PaymentError is an error used to communicate whether an error is temporary.
type PaymentError struct {
	OriginalError  error
	FailureMessage string
	Temporary      bool
}

// Error makes ProcessingError an error
func (e PaymentError) Error() string {
	msg := fmt.Sprintf("error: %s", e.FailureMessage)
	if e.Cause() != nil {
		msg = fmt.Sprintf("%s: %s", msg, e.Cause())
	}
	return msg
}

// Cause implements Cause for error
func (e PaymentError) Cause() error {
	return e.OriginalError
}

// Unwrap implements Unwrap for error
func (e PaymentError) Unwrap() error {
	return e.OriginalError
}

// ProcessingErrorFromError - given an error turn it into a processing error
func ProcessingErrorFromError(cause error, isTemporary bool) error {
	return &PaymentError{
		OriginalError:  cause,
		FailureMessage: cause.Error(),
		Temporary:      isTemporary,
	}
}

// GenerateIdempotencyKey returns a UUID v5 ID if the ID on the Transaction matches its expected value. Otherwise, it returns
// an error.
func (p *PaymentState) GenerateIdempotencyKey(namespace uuid.UUID) (*uuid.UUID, error) {
	authenticatedState, err := p.toAuthenticatedPaymentState()
	if err != nil {
		return nil, err
	}
	generatedIdempotencyKey := authenticatedState.generateIdempotencyKey(namespace)
	if generatedIdempotencyKey != *p.ID {
		return nil, fmt.Errorf("ID does not match transaction fields: have %s, want %s", *p.ID, generatedIdempotencyKey)
	}

	return p.ID, nil
}

// SetIdempotencyKey assigns a UUID v5 value to PaymentState.ID.
func (p *PaymentState) SetIdempotencyKey(namespace uuid.UUID) error {
	authenticatedPaymentState, err := p.toAuthenticatedPaymentState()
	if err != nil {
		return err
	}
	generatedIdempotencyKey := authenticatedPaymentState.generateIdempotencyKey(namespace)
	p.ID = &generatedIdempotencyKey
	return nil
}

var (
	prepareFailure = "prepare"
	submitFailure  = "submit"
)

// MarshalJSON - custom marshaling of transaction type.
func (a *AuthenticatedPaymentState) MarshalJSON() ([]byte, error) {
	type Alias AuthenticatedPaymentState
	return json.Marshal(&struct {
		Amount *decimal.Decimal `json:"amount"`
		*Alias
	}{
		Amount: &a.Amount,
		Alias:  (*Alias)(a),
	})
}

// UnmarshalJSON - custom unmarshal of transaction type.
func (a *AuthenticatedPaymentState) UnmarshalJSON(data []byte) error {
	type Alias AuthenticatedPaymentState
	aux := &struct {
		Amount *decimal.Decimal `json:"amount"`
		*Alias
	}{
		Alias: (*Alias)(a),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal transaction: %w", err)
	}
	if aux.Amount == nil {
		return fmt.Errorf("missing required transaction value: Amount")
	}
	a.Amount = *aux.Amount
	return nil
}

func (p *AuthenticatedPaymentState) generateIdempotencyKey(namespace uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(
		namespace,
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

func (t *PaymentState) getIdempotencyKey() *uuid.UUID {
	return t.ID
}

func (t *AuthenticatedPaymentState) nextStateValid(nextState PaymentStatus) bool {
	if t.Status == nextState {
		return true
	}
	// New transaction state should be present in the list of valid next states for the current
	// state.
	if !slices.Contains(t.Status.GetValidTransitions(), nextState) {
		return false
	}
	return true
}

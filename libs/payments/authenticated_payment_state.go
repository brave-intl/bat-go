package payments

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

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

type AuthenticatedPaymentState struct {
	PaymentDetails
	Status         PaymentStatus          `json:"status"`
	Authorizations []PaymentAuthorization `json:"authorizations"`
	DryRun         *string                `json:"dryRun"`
	LastError      *PaymentError          `json:"lastError"`
	DocumentID     string                 `json:"documentID"`
}

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

func (t *AuthenticatedPaymentState) NextStateValid(nextState PaymentStatus) bool {
	if t.Status == nextState {
		return true
	}
	// New transaction state should be present in the list of valid next states for the current
	// state.
	return statusListContainsStatus(t.Status.GetValidTransitions(), nextState)
}

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

func statusListContainsStatus(s []PaymentStatus, e PaymentStatus) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

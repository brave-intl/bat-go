package payments

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/exp/slices"
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
	Status         PaymentStatus
	Authorizations []PaymentAuthorization `json:"authorizations"`
	DryRun         *string                `json:"dryRun"`
	LastError      *PaymentError
	DocumentID     string
}

func (p *AuthenticatedPaymentState) GenerateIdempotencyKey() uuid.UUID {
	idempotencyNamespace := uuid.MustParse("3c0e75eb-9150-40b4-a988-a017d115de3c")
	return uuid.NewSHA1(
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
}

func (t *AuthenticatedPaymentState) NextStateValid(nextState PaymentStatus) bool {
	if t.Status == nextState {
		return true
	}
	// New transaction state should be present in the list of valid next states for the current
	// state.
	return slices.Contains(t.Status.GetValidTransitions(), nextState)
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

func AuthenticatedPaymentStateFromPaymentDetails(details PaymentDetails) AuthenticatedPaymentState {
	return AuthenticatedPaymentState{
		PaymentDetails: details,
	}
}

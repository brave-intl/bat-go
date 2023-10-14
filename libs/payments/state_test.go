package payments

import (
	"crypto"
	"encoding/json"
	"testing"
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

var (
	dID              = "HgXBNdOVAy83ZBy5oM26OE"
	generatedUUID, _ = uuid.Parse("727ccc14-1951-5a75-bbce-489505a684b1")
	amount           = decimal.NewFromFloat(1.1)
	txn0             = AuthenticatedPaymentState{Status: Prepared, PaymentDetails: PaymentDetails{Amount: amount}}
	txn1             = AuthenticatedPaymentState{DocumentID: "HgXBNdOVAy83ZBy5oM26OE", Status: Authorized, PaymentDetails: PaymentDetails{Amount: amount}}
	txn2             = AuthenticatedPaymentState{DocumentID: "HgXBNdOVAy83ZBy5oM26OE", Status: Pending, PaymentDetails: PaymentDetails{Amount: amount}}
	txn3             = AuthenticatedPaymentState{DocumentID: "HgXBNdOVAy83ZBy5oM26OE", Status: Paid, PaymentDetails: PaymentDetails{Amount: amount}}
	txn4             = AuthenticatedPaymentState{DocumentID: "HgXBNdOVAy83ZBy5oM26OE", Status: Failed, PaymentDetails: PaymentDetails{Amount: amount}}
	status0, _       = json.Marshal(txn0)
	status1, _       = json.Marshal(txn1)
	status2, _       = json.Marshal(txn2)
	status3, _       = json.Marshal(txn3)
	status4, _       = json.Marshal(txn4)
)

type mockVerifier struct {
	value bool
}

func (m mockVerifier) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	return m.value, nil
}

type mockKeystore struct{}

func (m mockKeystore) LookupVerifier(
	ctx context.Context,
	keyID string,
) (context.Context, *Verifier, error) {
	var verifier *Verifier = &mockVerifier{value: true}
	return ctx, verifier, nil
}


var transactionHistorySetTrue = []PaymentStateHistory{
	{
		{UnsafePaymentState: status0, ID: generatedUUID},
		{UnsafePaymentState: status1, ID: generatedUUID},
		{UnsafePaymentState: status2, ID: generatedUUID},
		{UnsafePaymentState: status3, ID: generatedUUID},
	},
	{
		{UnsafePaymentState: status0, ID: generatedUUID},
		{UnsafePaymentState: status1, ID: generatedUUID},
		{UnsafePaymentState: status2, ID: generatedUUID},
		{UnsafePaymentState: status4, ID: generatedUUID},
	},
	{
		{UnsafePaymentState: status0, ID: generatedUUID},
		{UnsafePaymentState: status1, ID: generatedUUID},
		{UnsafePaymentState: status2, ID: generatedUUID},
		{UnsafePaymentState: status4, ID: generatedUUID},
	},
	{
		{UnsafePaymentState: status0, ID: generatedUUID},
		{UnsafePaymentState: status1, ID: generatedUUID},
		{UnsafePaymentState: status4, ID: generatedUUID},
	},
	{
		{UnsafePaymentState: status0, ID: generatedUUID},
		{UnsafePaymentState: status4, ID: generatedUUID},
	},
	{
		{UnsafePaymentState: status0, ID: generatedUUID},
	},
}

var transactionHistorySetFalse = []PaymentStateHistory{
	// Transitions must always start at 0
	{
		{UnsafePaymentState: status1, ID: generatedUUID},
	},
	{
		{UnsafePaymentState: status2, ID: generatedUUID},
	},
	{
		{UnsafePaymentState: status3, ID: generatedUUID},
	},
	{
		{UnsafePaymentState: status4, ID: generatedUUID},
	},
	{
		{UnsafePaymentState: status3, ID: generatedUUID},
		{UnsafePaymentState: status4, ID: generatedUUID},
	},
	{
		{UnsafePaymentState: status0, ID: generatedUUID},
		{UnsafePaymentState: status3, ID: generatedUUID},
	},
	{
		{UnsafePaymentState: status0, ID: generatedUUID},
		{UnsafePaymentState: status2, ID: generatedUUID},
	},
}

func TestIdempotencyKey(t *testing.T) {
	details := PaymentDetails{
		Amount:    decimal.NewFromFloat(12.234),
		To:        "683bc9ba-497a-47a5-9587-3bd03fd722bd",
		From:      "af68d02a-907f-4e9a-8f74-b54c7629412b",
		Custodian: "uphold",
		PayoutID:  "78910",
	}
	assert.Equal(
		t,
		details.IdempotencyKey().String(),
		"29ccbbfd-7a77-5874-a5bb-d043d9f38bf2",
	)
}

func TestGetAuthenticatedPaymentState(t *testing.T) {
	keystore := mockKeystore{}
	// Valid transitions should be valid
	for _, transactionHistorySet := range transactionHistorySetTrue {
		authenticatedState, err := transactionHistorySet.GetAuthenticatedPaymentState(
			keystore,
			dID,
		)
		assert.Nil(t, err)
		assert.NotNil(t, authenticatedState)
	}

	// Invalid transitions should be invalid
	for _, transactionHistorySet := range transactionHistorySetFalse {
		authenticatedState, err := transactionHistorySet.GetAuthenticatedPaymentState(
			keystore,
			dID,
		)
		assert.Error(t, err)
		assert.Nil(t, authenticatedState)
	}

	// Valid transitions should be invalid with wrong doc id
	for _, transactionHistorySet := range transactionHistorySetTrue {
		authenticatedState, err := transactionHistorySet.GetAuthenticatedPaymentState(
			keystore,
			"Cn7mea9LNqEH6FWK1XX38o",
		)
		// initial state does not have an embedded document id to cause mismatch
		if len(transactionHistorySet) > 1 {
			assert.Error(t, err)
			assert.Nil(t, authenticatedState)
		}
	}

	verifier.value = false
	// Valid transitions should be invalid with bad signatures
	for _, transactionHistorySet := range transactionHistorySetTrue {
		authenticatedState, err := transactionHistorySet.GetAuthenticatedPaymentState(
			keystore,
			dID,
		)
		assert.Error(t, err)
		assert.Nil(t, authenticatedState)
	}
}

package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/libs/clients/gemini"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

var (
	mockGeminiHost           = "fake://mock.gemini.com"
	geminiSucceedTransaction = custodian.Transaction{ProviderID: "1234"}
	// geminiFailTransaction    = custodian.Transaction{ProviderID: "1234"}
	geminiBulkPayload = gemini.BulkPayoutPayload{
		OauthClientID: "",
		Payouts:       []gemini.PayoutPayload{},
	}
)

/*
TestGeminiStateMachineHappyPathTransitions tests for correct state progression from
Initialized to Paid. Additionally, Paid status should be final and Failed status should
be permanent.
*/
func TestGeminiStateMachineHappyPathTransitions(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	os.Setenv("GEMINI_ENVIRONMENT", "test")

	// Mock transaction creation
	jsonResponse, err := json.Marshal(geminiBulkPaySuccessResponse)
	if err != nil {
		panic(err)
	}
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/v1/payments/bulkPay",
			mockGeminiHost,
		),
		httpmock.NewStringResponder(200, string(jsonResponse)),
	)
	// Mock transaction commit that will succeed
	jsonResponse, err = json.Marshal(geminiTransactionCheckSuccessResponse)
	if err != nil {
		panic(err)
	}
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/v1/payment/%s/%s", // fmt.Sprintf("/v1/payment/%s/%s", clientID, txRef)
			mockGeminiHost,
			geminiBulkPayload.OauthClientID,
			geminiSucceedTransaction.ProviderID,
		),
		httpmock.NewStringResponder(200, string(jsonResponse)),
	)

	ctx := context.Background()
	geminiStateMachine := GeminiMachine{}
	mockDriver := new(mockDriver)
	transaction := Transaction{State: Initialized}

	// Should create a transaction in QLDB. Current state argument is empty because
	// the object does not yet exist.
	newState, _ := Drive(ctx, &geminiStateMachine, &transaction, mockDriver)
	assert.Equal(t, Initialized, newState)

	// Create a sample state to represent the now-initialized entity.
	transaction.State = Prepared

	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")

	// Should transition transaction into the Authorized state
	newState, _ = Drive(ctx, &geminiStateMachine, &transaction, mockDriver)
	assert.Equal(t, Authorized, newState)

	transaction.State = Authorized
	// Should transition transaction into the Pending state
	newState, _ = Drive(ctx, &geminiStateMachine, &transaction, mockDriver)
	assert.Equal(t, Pending, newState)

	transaction.State = Pending
	// Should transition transaction into the Paid state
	newState, _ = Drive(ctx, &geminiStateMachine, &transaction, mockDriver)
	assert.Equal(t, Paid, newState)

	transaction.State = Paid
	// Should transition transaction into the Authorized state when the payment fails
	newState, _ = Drive(ctx, &geminiStateMachine, &transaction, mockDriver)
	assert.Equal(t, Paid, newState)

	transaction.State = Failed
	// Should transition transaction into the Authorized state when the payment fails
	newState, _ = Drive(ctx, &geminiStateMachine, &transaction, mockDriver)
	assert.Equal(t, Failed, newState)
}

/*
TestGeminiStateMachine500FailureToPendingTransitions tests for a failure to progress status
after a 500 error response while attempting to transfer from Pending to Paid
*/
func TestGeminiStateMachine500FailureToPendingTransitions(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	mockGeminiHost := "https://mock.gemini.com"
	err := os.Setenv("GEMINI_ENVIRONMENT", "test")
	if err != nil {
		panic("failed to set environment variable")
	}

	// Mock transaction creation that will fail
	jsonResponse, err := json.Marshal(geminiBulkPayFailureResponse)
	if err != nil {
		panic(err)
	}
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/v1/payments/bulkPay",
			mockGeminiHost,
		),
		httpmock.NewStringResponder(500, string(jsonResponse)),
	)

	ctx := context.Background()
	mockDriver := new(mockDriver)
	transaction := Transaction{State: Authorized}
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	geminiStateMachine := GeminiMachine{}
	// When the implementation is in place, this Version value will not be necessary.
	// However, it's set here to allow the placeholder implementation to return the
	// correct value and allow this test to pass in the mean time.
	// @TODO: Make this test fail
	// currentVersion := 500

	// Should transition transaction into the Paid state
	newState, _ := Drive(ctx, &geminiStateMachine, &transaction, mockDriver)
	assert.Equal(t, Authorized, newState)
}

/*
TestGeminiStateMachine404FailureToPaidTransitions tests for a failure to progress status
Failure with 404 error when attempting to transfer from Pending to Paid
*/
func TestGeminiStateMachine404FailureToPaidTransitions(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	mockGeminiHost := "https://mock.gemini.com"
	err := os.Setenv("GEMINI_ENVIRONMENT", "test")
	if err != nil {
		panic("failed to set environment variable")
	}

	var (
		failTransaction   = custodian.Transaction{ProviderID: "1234"}
		geminiBulkPayload = gemini.BulkPayoutPayload{
			OauthClientID: "",
			Payouts:       []gemini.PayoutPayload{},
		}
	)

	// Mock transaction commit that will fail
	jsonResponse, err := json.Marshal(geminiTransactionCheckFailureResponse)
	if err != nil {
		panic(err)
	}
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/v1/payment/%s/%s", // fmt.Sprintf("/v1/payment/%s/%s", clientID, txRef)
			mockGeminiHost,
			geminiBulkPayload.OauthClientID,
			failTransaction.ProviderID,
		),
		httpmock.NewStringResponder(404, string(jsonResponse)),
	)

	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	mockDriver := new(mockDriver)
	transaction := Transaction{State: Pending}
	// When the implementation is in place, this Version value will not be necessary.
	// However, it's set here to allow the placeholder implementation to return the
	// correct value and allow this test to pass in the mean time.
	// @TODO: Make this test fail
	// currentVersion := 404
	geminiStateMachine := GeminiMachine{}

	// Should transition transaction into the Paid state
	newState, _ := Drive(ctx, &geminiStateMachine, &transaction, mockDriver)
	assert.Equal(t, Pending, newState)
}

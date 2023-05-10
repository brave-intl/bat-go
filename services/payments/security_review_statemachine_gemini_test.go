package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/google/uuid"

	"github.com/brave-intl/bat-go/libs/clients/gemini"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

	err := os.Setenv("GEMINI_ENVIRONMENT", "test")
	if err != nil {
		panic(err)
	}

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

	namespaceUUID, err := uuid.Parse("7478bd8a-2247-493d-b419-368f1a1d7a6c")
	idempotencyKey, err := uuid.Parse("6798046b-2d05-5df4-9e18-fb3caf1b583d")
	if err != nil {
		panic(err)
	}
	geminiStateMachine := GeminiMachine{}

	testTransaction := Transaction{
		State: Prepared,
		ID:    &idempotencyKey,
	}

	marshaledData, err := ion.MarshalBinary(testTransaction)
	if err != nil {
		panic(err)
	}
	mockTransitionHistory := QLDBPaymentTransitionHistoryEntry{
		BlockAddress: qldbPaymentTransitionHistoryEntryBlockAddress{
			StrandID:   "test",
			SequenceNo: 1,
		},
		Hash: "test",
		Data: QLDBPaymentTransitionHistoryEntryData{
			Data:           marshaledData,
			Signature:      []byte{},
			IdempotencyKey: &idempotencyKey,
		},
		Metadata: QLDBPaymentTransitionHistoryEntryMetadata{
			ID:      "test",
			Version: 1,
			TxTime:  time.Now(),
			TxID:    "test",
		},
	}
	mockKMS := new(mockKMSClient)
	mockDriver := new(mockDriver)
	mockKMS.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return(&kms.SignOutput{Signature: []byte("succeed")}, nil)
	mockKMS.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(&kms.VerifyOutput{SignatureValid: true}, nil)
	mockKMS.On("GetPublicKey", mock.Anything, mock.Anything, mock.Anything).Return(&kms.GetPublicKeyOutput{PublicKey: []byte("test")}, nil)

	service := Service{
		datastore:        mockDriver,
		kmsSigningClient: mockKMS,
		baseCtx:          context.Background(),
	}
	geminiStateMachine.setService(&service)
	geminiStateMachine.setTransaction(&testTransaction)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "namespaceUUID", namespaceUUID)
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")

	// Should create a transaction in QLDB. Current state argument is empty because
	// the object does not yet exist.
	mockCall := mockDriver.On("Execute", mock.Anything, mock.Anything).Return(nil, nil)
	newTransaction, err := Drive(ctx, &geminiStateMachine)
	if err != nil {
		panic(fmt.Sprintf("Preparing: %e", err))
	}
	assert.Equal(t, Prepared, newTransaction.State)

	// Should transition transaction into the Authorized state
	testTransaction.State = Prepared
	marshaledData, _ = ion.MarshalBinary(testTransaction)
	mockTransitionHistory.Data.Data = marshaledData
	geminiStateMachine.setTransaction(&testTransaction)
	mockCall.Unset()
	mockCall = mockDriver.On("Execute", mock.Anything, mock.Anything).Return(&mockTransitionHistory, nil)
	newTransaction, err = Drive(ctx, &geminiStateMachine)
	if err != nil {
		panic(fmt.Sprintf("Authorizing: %e", err))
	}
	assert.Equal(t, Authorized, newTransaction.State)

	// Should transition transaction into the Pending state
	testTransaction.State = Authorized
	marshaledData, _ = ion.MarshalBinary(testTransaction)
	mockTransitionHistory.Data.Data = marshaledData
	geminiStateMachine.setTransaction(&testTransaction)
	mockCall.Unset()
	mockCall = mockDriver.On("Execute", mock.Anything, mock.Anything).Return(&mockTransitionHistory, nil)
	newTransaction, err = Drive(ctx, &geminiStateMachine)
	if err != nil {
		panic(fmt.Sprintf("Pending: %e", err))
	}
	assert.Equal(t, Pending, newTransaction.State)

	// Should transition transaction into the Paid state
	testTransaction.State = Pending
	marshaledData, _ = ion.MarshalBinary(testTransaction)
	mockTransitionHistory.Data.Data = marshaledData
	geminiStateMachine.setTransaction(&testTransaction)
	mockCall.Unset()
	mockCall = mockDriver.On("Execute", mock.Anything, mock.Anything).Return(&mockTransitionHistory, nil)
	newTransaction, err = Drive(ctx, &geminiStateMachine)
	if err != nil {
		panic(fmt.Sprintf("Paying: %e", err))
	}
	assert.Equal(t, Paid, newTransaction.State)
}

/*
TestGeminiStateMachine500FailureToPendingTransitions tests for a failure to progress status
after a 500 error response while attempting to transfer from Pending to Paid
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
	service := Service{
		datastore: mockDriver,
		baseCtx:   context.Background(),
	}
	id := uuid.New()
	transaction := Transaction{State: Authorized, ID: &id}
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	geminiStateMachine := GeminiMachine{}
	geminiStateMachine.setTransaction(&transaction)
	geminiStateMachine.setService(&service)
	// When the implementation is in place, this Version value will not be necessary.
	// However, it's set here to allow the placeholder implementation to return the
	// correct value and allow this test to pass in the mean time.
	// @TODO: Make this test fail
	// currentVersion := 500

	// Should transition transaction into the Paid state
	newState, _ := Drive(ctx, &geminiStateMachine)
	assert.Equal(t, Authorized, newState)
}
*/

/*
TestGeminiStateMachine404FailureToPaidTransitions tests for a failure to progress status
Failure with 404 error when attempting to transfer from Pending to Paid
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
	service := Service{
		datastore: mockDriver,
		baseCtx:   context.Background(),
	}
	id := uuid.New()
	transaction := Transaction{State: Pending, ID: &id}
	// When the implementation is in place, this Version value will not be necessary.
	// However, it's set here to allow the placeholder implementation to return the
	// correct value and allow this test to pass in the mean time.
	// @TODO: Make this test fail
	// currentVersion := 404
	geminiStateMachine := GeminiMachine{}
	geminiStateMachine.setTransaction(&transaction)
	geminiStateMachine.setService(&service)

	// Should transition transaction into the Paid state
	newState, _ := Drive(ctx, &geminiStateMachine)
	assert.Equal(t, Pending, newState)
}
*/

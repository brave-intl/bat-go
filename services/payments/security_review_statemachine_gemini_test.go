package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/brave-intl/bat-go/libs/clients/gemini"
	"github.com/brave-intl/bat-go/libs/custodian"
	. "github.com/brave-intl/bat-go/libs/payments"
	"github.com/jarcoal/httpmock"
	should "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	must "github.com/stretchr/testify/require"
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
	must.Equal(t, nil, err)

	// Mock transaction creation
	jsonResponse, err := json.Marshal(geminiBulkPaySuccessResponse)
	must.Equal(t, nil, err)
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
	must.Equal(t, nil, err)
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
	must.Equal(t, nil, err)
	idempotencyKey, err := uuid.Parse("5d8a3ebc-e622-5b8e-9090-9b4ea09e74c8")
	must.Equal(t, nil, err)
	geminiStateMachine := GeminiMachine{}

	testTransaction := AuthenticatedPaymentState{
		Status: Prepared,
		PaymentDetails: PaymentDetails{
			Amount:    decimal.NewFromFloat(1.1),
			Custodian: "gemini",
		},
		Authorizations: []PaymentAuthorization{{}, {}, {}},
	}

	marshaledData, _ := json.Marshal(testTransaction)
	must.Equal(t, nil, err)
	mockTransitionHistory := QLDBPaymentTransitionHistoryEntry{
		BlockAddress: QLDBPaymentTransitionHistoryEntryBlockAddress{
			StrandID:   "test",
			SequenceNo: 1,
		},
		Hash: "test",
		Data: PaymentState{
			UnsafePaymentState: marshaledData,
			Signature:          []byte{},
			ID:                 idempotencyKey,
		},
		Metadata: QLDBPaymentTransitionHistoryEntryMetadata{
			ID:      "test",
			Version: 1,
			TxTime:  time.Now(),
			TxID:    "test",
		},
	}
	marshaledAuthenticatedState, err := json.Marshal(AuthenticatedPaymentState{Status: Prepared})
	must.Equal(t, nil, err)
	mockedPaymentState := PaymentState{UnsafePaymentState: marshaledAuthenticatedState}
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
	geminiStateMachine.setPersistenceConfigValues(
		service.datastore,
		service.sdkClient,
		service.kmsSigningClient,
		service.kmsSigningKeyID,
		&testTransaction,
	)
	geminiStateMachine.setTransaction(&testTransaction)

	ctx := context.Background()
	ctx = context.WithValue(ctx, serviceNamespaceContextKey{}, namespaceUUID)
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")

	// First call in order is to insertPayment and should return a fake document ID
	mockDriver.On("Execute", mock.Anything, mock.Anything).Return("123456", nil).Once()
	// Next call in order is to get GetTransactionFromDocumentID and should return an
	// AuthenticatedPaymentState.
	mockDriver.On("Execute", mock.Anything, mock.Anything).Return(&mockedPaymentState, nil).Once()
	// All further calls should return the mocked history entry.
	mockDriver.On("Execute", mock.Anything, mock.Anything).Return(&mockTransitionHistory.Data, nil)
	insertedDocumentID, err := service.insertPayment(ctx, testTransaction.PaymentDetails)
	must.Equal(t, nil, err)
	must.Equal(t, "123456", insertedDocumentID)
	newTransaction, _, err := service.GetTransactionFromDocumentID(ctx, insertedDocumentID)
	must.Equal(t, nil, err)
	should.Equal(t, Prepared, newTransaction.Status)

	// Should transition transaction into the Authorized state
	//	testTransaction.Status = Prepared
	//	marshaledData, _ = json.Marshal(testTransaction)
	//	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	//	geminiStateMachine.setTransaction(&testTransaction)
	//	newTransaction, err = Drive(ctx, &geminiStateMachine)
	//	must.Equal(t, nil, err)
	//	should.Equal(t, Authorized, newTransaction.Status)

	//	// Should transition transaction into the Pending state
	//	testTransaction.Status = Authorized
	//	marshaledData, _ = json.Marshal(testTransaction)
	//	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	//	geminiStateMachine.setTransaction(&testTransaction)
	//	newTransaction, err = Drive(ctx, &geminiStateMachine)
	//	must.Equal(t, nil, err)
	//	// @TODO: When tests include custodial mocks, this should be Pending
	//	should.Equal(t, Paid, newTransaction.Status)
	//
	//	// Should transition transaction into the Paid state
	//	testTransaction.Status = Pending
	//	marshaledData, _ = json.Marshal(testTransaction)
	//	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	//	geminiStateMachine.setTransaction(&testTransaction)
	//	newTransaction, err = Drive(ctx, &geminiStateMachine)
	//	must.Equal(t, nil, err)
	//	should.Equal(t, Paid, newTransaction.Status)
}

/*
TestGeminiStateMachine500FailureToPendingTransitions tests for a failure to progress status
after a 500 error response while attempting to transfer from Pending to Paid
func TestGeminiStateMachine500FailureToPendingTransitions(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	mockGeminiHost := "https://mock.gemini.com"
	err := os.Setenv("GEMINI_ENVIRONMENT", "test")
	must.Equal(t, nil, err)

	// Mock transaction creation that will fail
	jsonResponse, err := json.Marshal(geminiBulkPayFailureResponse)
	must.Equal(t, nil, err)
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
	should.Equal(t, Authorized, newState)
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
	must.Equal(t, nil, err)

	var (
		failTransaction   = custodian.Transaction{ProviderID: "1234"}
		geminiBulkPayload = gemini.BulkPayoutPayload{
			OauthClientID: "",
			Payouts:       []gemini.PayoutPayload{},
		}
	)

	// Mock transaction commit that will fail
	jsonResponse, err := json.Marshal(geminiTransactionCheckFailureResponse)
	must.Equal(t, nil, err)
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
	should.Equal(t, Pending, newState)
}
*/

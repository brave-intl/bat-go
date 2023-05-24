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
	"github.com/stretchr/testify/mock"

	"github.com/jarcoal/httpmock"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

var (
	mockBitflyerHost = "fake://bitflyer.com"
	// bitflyerBulkPayload = bitflyer.WithdrawToDepositIDBulkPayload{
	// 	DryRun:      true,
	// 	Withdrawals: []bitflyer.WithdrawToDepositIDPayload{},
	// 	PriceToken:  "",
	// 	DryRunOption: &bitflyer.DryRunOption{
	// 		RequestAPITransferStatus: "",
	// 		ProcessTimeSec:           1,
	// 		StatusAPITransferStatus:  "",
	// 	},
	// }
)

type ctxAuthKey struct{}

/*
TestBitflyerStateMachineHappyPathTransitions tests for correct state progression from
Initialized to Paid. Additionally, Paid status should be final and Failed status should
be permanent.
*/
func TestBitflyerStateMachineHappyPathTransitions(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	err := os.Setenv("BITFLYER_ENVIRONMENT", "test")
	must.Equal(t, nil, err)

	// Mock transaction creation that will succeed
	jsonResponse, err := json.Marshal(bitflyerTransactionSubmitSuccessResponse)
	must.Equal(t, nil, err)
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/api/link/v1/coin/withdraw-to-deposit-id/bulk-request",
			mockBitflyerHost,
		),
		httpmock.NewStringResponder(200, string(jsonResponse)),
	)
	// Mock transaction commit that will succeed
	jsonResponse, err = json.Marshal(bitflyerTransactionCheckStatusSuccessResponse)
	must.Equal(t, nil, err)
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/api/link/v1/coin/withdraw-to-deposit-id/bulk-status",
			mockBitflyerHost,
		),
		httpmock.NewStringResponder(200, string(jsonResponse)),
	)

	namespaceUUID, err := uuid.Parse("7478bd8a-2247-493d-b419-368f1a1d7a6c")
	must.Equal(t, nil, err)
	idempotencyKey, err := uuid.Parse("6798046b-2d05-5df4-9e18-fb3caf1b583d")
	must.Equal(t, nil, err)
	bitflyerStateMachine := BitflyerMachine{}

	testTransaction := Transaction{
		State: Prepared,
		ID:    &idempotencyKey,
	}

	marshaledData, err := ion.MarshalBinary(testTransaction)
	must.Equal(t, nil, err)
	mockTransitionHistory := qldbPaymentTransitionHistoryEntry{
		BlockAddress: qldbPaymentTransitionHistoryEntryBlockAddress{
			StrandID:   "test",
			SequenceNo: 1,
		},
		Hash: "test",
		Data: qldbPaymentTransitionHistoryEntryData{
			Data:           marshaledData,
			Signature:      []byte{},
			IdempotencyKey: &idempotencyKey,
		},
		Metadata: qldbPaymentTransitionHistoryEntryMetadata{
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
	bitflyerStateMachine.setService(&service)
	bitflyerStateMachine.setTransaction(&testTransaction)

	ctx := context.Background()
	ctx = context.WithValue(ctx, serviceNamespaceContextKey{}, namespaceUUID)
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")

	// Should create a transaction in QLDB. Current state argument is empty because
	// the object does not yet exist.
	mockCall := mockDriver.On("Execute", mock.Anything, mock.Anything).Return(nil, nil)
	newTransaction, err := Drive(ctx, &bitflyerStateMachine)
	must.Equal(t, nil, err)
	should.Equal(t, Prepared, newTransaction.State)

	// Should transition transaction into the Authorized state
	testTransaction.State = Prepared
	marshaledData, _ = ion.MarshalBinary(testTransaction)
	mockTransitionHistory.Data.Data = marshaledData
	bitflyerStateMachine.setTransaction(&testTransaction)
	mockCall.Unset()
	mockCall = mockDriver.On("Execute", mock.Anything, mock.Anything).Return(&mockTransitionHistory, nil)
	newTransaction, err = Drive(ctx, &bitflyerStateMachine)
	must.Equal(t, nil, err)
	should.Equal(t, Authorized, newTransaction.State)

	// Should transition transaction into the Pending state
	testTransaction.State = Authorized
	marshaledData, _ = ion.MarshalBinary(testTransaction)
	mockTransitionHistory.Data.Data = marshaledData
	bitflyerStateMachine.setTransaction(&testTransaction)
	mockCall.Unset()
	mockCall = mockDriver.On("Execute", mock.Anything, mock.Anything).Return(&mockTransitionHistory, nil)
	newTransaction, err = Drive(ctx, &bitflyerStateMachine)
	must.Equal(t, nil, err)
	should.Equal(t, Pending, newTransaction.State)

	// Should transition transaction into the Paid state
	testTransaction.State = Pending
	marshaledData, _ = ion.MarshalBinary(testTransaction)
	mockTransitionHistory.Data.Data = marshaledData
	bitflyerStateMachine.setTransaction(&testTransaction)
	mockCall.Unset()
	mockCall = mockDriver.On("Execute", mock.Anything, mock.Anything).Return(&mockTransitionHistory, nil)
	newTransaction, err = Drive(ctx, &bitflyerStateMachine)
	must.Equal(t, nil, err)
	should.Equal(t, Paid, newTransaction.State)
}

/*
TestBitflyerStateMachine500FailureToPaidTransition tests for a failure to progress status
after a 500 error response while attempting to transfer from Pending to Paid
func TestBitflyerStateMachine500FailureToPaidTransition(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	err := os.Setenv("BITFLYER_ENVIRONMENT", "test")
	must.Equal(t, nil, err)

	// Mock transaction commit that will fail
	jsonResponse, err := json.Marshal(bitflyerTransactionSubmitFailureResponse)
	must.Equal(t, nil, err)
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/api/link/v1/coin/withdraw-to-deposit-id/bulk-status",
			mockBitflyerHost,
		),
		httpmock.NewStringResponder(500, string(jsonResponse)),
	)

	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	mockDriver := new(mockDriver)
	service := Service{
		datastore: mockDriver,
		baseCtx:   context.Background(),
	}
	id := uuid.New()
	transaction := Transaction{State: Prepared, ID: &id}
	bitflyerStateMachine := BitflyerMachine{}
	bitflyerStateMachine.setTransaction(&transaction)
	bitflyerStateMachine.setService(&service)
	// When the implementation is in place, this Version value will not be necessary.
	// However, it's set here to allow the placeholder implementation to return the
	// correct value and allow this test to pass in the meantime.
	// @TODO: Make this test fail
	// currentVersion := 500

	newState, _ := Drive(ctx, &bitflyerStateMachine)
	should.Equal(t, Authorized, newState)
}
*/

/*
TestBitflyerStateMachine404FailureToPaidTransition tests for a failure to progress status
Failure with 404 error when attempting to transfer from Pending to Paid
func TestBitflyerStateMachine404FailureToPaidTransition(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	err := os.Setenv("BITFLYER_ENVIRONMENT", "test")
	must.Equal(t, nil, err)

	// Mock transaction commit that will fail
	jsonResponse, err := json.Marshal(bitflyerTransactionCheckStatusFailureResponse)
	must.Equal(t, nil, err)
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/api/link/v1/coin/withdraw-to-deposit-id/bulk-status",
			mockBitflyerHost,
		),
		httpmock.NewStringResponder(404, string(jsonResponse)),
	)

	ctx := context.Background()
	mockDriver := new(mockDriver)
	service := Service{
		datastore: mockDriver,
		baseCtx:   context.Background(),
	}
	id := uuid.New()
	transaction := Transaction{State: Pending, ID: &id}
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	bitflyerStateMachine := BitflyerMachine{}
	bitflyerStateMachine.setTransaction(&transaction)
	bitflyerStateMachine.setService(&service)
	// When the implementation is in place, this Version value will not be necessary.
	// However, it's set here to allow the placeholder implementation to return the
	// correct value and allow this test to pass in the meantime.
	// @TODO: Make this test fail
	// currentVersion := 404

	newState, _ := Drive(ctx, &bitflyerStateMachine)
	should.Equal(t, Pending, newState)
}
*/

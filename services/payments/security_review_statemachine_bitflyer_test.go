package payments
//
//import (
//	"context"
//	"crypto/ecdsa"
//	"crypto/elliptic"
//	"crypto/rand"
//	"crypto/x509"
//	"encoding/json"
//	"errors"
//	"fmt"
//	"net/http"
//	"os"
//	"testing"
//	"time"
//
//	"github.com/shopspring/decimal"
//
//	"github.com/aws/aws-sdk-go-v2/service/kms"
//	"github.com/google/uuid"
//	"github.com/stretchr/testify/mock"
//
//	//bitflyercmd "github.com/brave-intl/bat-go/tools/settlement/cmd"
//
//	paymentLib "github.com/brave-intl/bat-go/libs/payments"
//	"github.com/jarcoal/httpmock"
//	should "github.com/stretchr/testify/assert"
//	must "github.com/stretchr/testify/require"
//)
//
//var (
//	mockBitflyerHost = "http://bravesoftware.com"
//	// bitflyerBulkPayload = bitflyer.WithdrawToDepositIDBulkPayload{
//	// 	DryRun:      true,
//	// 	Withdrawals: []bitflyer.WithdrawToDepositIDPayload{},
//	// 	PriceToken:  "",
//	// 	DryRunOption: &bitflyer.DryRunOption{
//	// 		RequestAPITransferStatus: "",
//	// 		ProcessTimeSec:           1,
//	// 		StatusAPITransferStatus:  "",
//	// 	},
//	// }
//)
//
//type ctxAuthKey struct{}
//
///*
//TestBitflyerStateMachineHappyPathTransitions tests for correct state progression from
//Initialized to Paid. Additionally, Paid status should be final and Failed status should
//be permanent.
//*/
//func TestBitflyerStateMachineHappyPathTransitions(t *testing.T) {
//	err := os.Setenv("BITFLYER_ENVIRONMENT", "test")
//	must.Equal(t, nil, err)
//	err = os.Setenv("BITFLYER_SERVER", mockBitflyerHost)
//	must.Equal(t, nil, err)
//
//	bitflyerStateMachine := BitflyerMachine{
//		client:       http.Client{},
//		bitflyerHost: mockBitflyerHost,
//	}
//
//	httpmock.Activate()
//	defer httpmock.DeactivateAndReset()
//
//	// Mock transaction creation that will succeed
//	jsonSumbitaSuccessResponse, err := json.Marshal(bitflyerTransactionSubmitSuccessResponse)
//	must.Equal(t, nil, err)
//	httpmock.RegisterResponder(
//		"POST",
//		fmt.Sprintf(
//			"%s/api/link/v1/coin/withdraw-to-deposit-id/bulk-request",
//			mockBitflyerHost,
//		),
//		httpmock.NewStringResponder(200, string(jsonSumbitaSuccessResponse)),
//	)
//	// Mock transaction status check that will stay stuck in pending
//	jsonCheckStatusResponsePending, err := json.Marshal(bitflyerTransactionCheckStatusSuccessResponsePending)
//	must.Equal(t, nil, err)
//	httpmock.RegisterResponder(
//		"POST",
//		fmt.Sprintf(
//			"%s/api/link/v1/coin/withdraw-to-deposit-id/bulk-status",
//			mockBitflyerHost,
//		),
//		httpmock.NewStringResponder(200, string(jsonCheckStatusResponsePending)),
//	)
//	jsonTokenRefreshResponse, err := json.Marshal(bitflyerTransactionTokenRefreshResponse)
//	must.Equal(t, nil, err)
//	httpmock.RegisterResponder(
//		"POST",
//		fmt.Sprintf(
//			"%s/api/link/v1/token",
//			mockBitflyerHost,
//		),
//		httpmock.NewStringResponder(200, string(jsonTokenRefreshResponse)),
//	)
//	jsonPriceFetchResponse, err := json.Marshal(bitflyerFetchPriceResponse)
//	must.Equal(t, nil, err)
//	httpmock.RegisterResponder(
//		"GET",
//		fmt.Sprintf(
//			"%s/api/link/v1/getprice",
//			mockBitflyerHost,
//		),
//		httpmock.NewStringResponder(200, string(jsonPriceFetchResponse)),
//	)
//
//	namespaceUUID, err := uuid.Parse("7478bd8a-2247-493d-b419-368f1a1d7a6c")
//	must.Equal(t, nil, err)
//	idempotencyKey, err := uuid.Parse("1803df27-f29c-537a-9384-bb5b523ea3f7")
//	must.Equal(t, nil, err)
//
//	testTransaction := paymentLib.AuthenticatedPaymentState{
//		Status: paymentLib.Prepared,
//		PaymentDetails: paymentLib.PaymentDetails{
//			Amount:    decimal.NewFromFloat(1.1),
//			Custodian: "bitflyer",
//		},
//		Authorizations: []paymentLib.PaymentAuthorization{{}, {}, {}},
//	}
//
//	marshaledData, _ := json.Marshal(testTransaction)
//	must.Equal(t, nil, err)
//	privkey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
//	must.Equal(t, nil, err)
//	marshalledPubkey, err := x509.MarshalPKIXPublicKey(&privkey.PublicKey)
//	must.Nil(t, err)
//	mockTransitionHistory := QLDBPaymentTransitionHistoryEntry{
//		BlockAddress: QLDBPaymentTransitionHistoryEntryBlockAddress{
//			StrandID:   "test",
//			SequenceNo: 1,
//		},
//		Hash: []byte("test"),
//		Data: paymentLib.PaymentState{
//			UnsafePaymentState: marshaledData,
//			Signature:          []byte{},
//			ID:                 idempotencyKey,
//			PublicKey:          marshalledPubkey,
//		},
//		Metadata: QLDBPaymentTransitionHistoryEntryMetadata{
//			ID:      "test",
//			Version: 1,
//			TxTime:  time.Now(),
//			TxID:    "test",
//		},
//	}
//	//	marshaledAuthenticatedState, err := json.Marshal(AuthenticatedPaymentState{Status: Prepared})
//	//	must.Equal(t, nil, err)
//	//	mockedPaymentState := PaymentState{UnsafePaymentState: marshaledAuthenticatedState}
//	mockKMS := new(mockKMSClient)
//	mockDriver := new(mockDriver)
//	mockKMS.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return(&kms.SignOutput{Signature: []byte("succeed")}, nil)
//	mockKMS.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(&kms.VerifyOutput{SignatureValid: true}, nil)
//	mockKMS.On("GetPublicKey", mock.Anything, mock.Anything, mock.Anything).Return(
//		&kms.GetPublicKeyOutput{
//			PublicKey: marshalledPubkey,
//		},
//		nil,
//	)
//
//	//service := Service{
//	//datastore:        mockDriver,
//	//kmsSigningClient: mockKMS,
//	//baseCtx:          context.Background(),
//	//}
//	//bitflyerStateMachine.setPersistenceConfigValues(
//	//service.datastore,
//	//service.sdkClient,
//	//service.kmsSigningClient,
//	//service.kmsSigningKeyID,
//	bitflyerStateMachine.setTransaction(
//		&testTransaction,
//	)
//
//	ctx := context.Background()
//	ctx = context.WithValue(ctx, serviceNamespaceContextKey{}, namespaceUUID)
//	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
//
//	// First call in order is to insertPayment and should return a fake document ID
//	mockDriver.On("Execute", mock.Anything, mock.Anything).Return("123456", nil).Once()
//	// Next call in order is to get GetTransactionFromDocumentID and should return an
//	// AuthenticatedPaymentState.
//	mockDriver.On("Execute", mock.Anything, mock.Anything).Return(&testTransaction, nil).Once()
//	// All further calls should return the mocked history entry.
//	mockDriver.On("Execute", mock.Anything, mock.Anything).Return(&testTransaction, nil)
//	//insertedDocumentID, err := service.insertPayment(ctx, testTransaction.PaymentDetails)
//	//must.Equal(t, nil, err)
//	//must.Equal(t, "123456", insertedDocumentID)
//	//newTransaction, _, err := service.GetTransactionFromDocumentID(ctx, insertedDocumentID)
//	//must.Equal(t, nil, err)
//	//should.Equal(t, Prepared, newTransaction.Status)
//
//	// Should transition transaction into the Authorized state
//	testTransaction.Status = paymentLib.Prepared
//	marshaledData, _ = json.Marshal(testTransaction)
//	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
//	bitflyerStateMachine.setTransaction(&testTransaction)
//	newTransaction, err := Drive(ctx, &bitflyerStateMachine)
//	must.Equal(t, nil, err)
//	info := httpmock.GetCallCountInfo()
//	tokenInfoKey := fmt.Sprintf("POST %s/api/link/v1/token", mockBitflyerHost)
//	fmt.Printf("Calls to token refresh: %v\n", info[tokenInfoKey])
//	// Ensure that our Bitflyer calls are going through the mock and not anything else.
//	//must.Equal(t, info[tokenInfoKey], 1)
//	should.Equal(t, paymentLib.Authorized, newTransaction.Status)
//
//	// Should transition transaction into the Pending state
//	testTransaction.Status = paymentLib.Authorized
//	marshaledData, _ = json.Marshal(testTransaction)
//	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
//	bitflyerStateMachine.setTransaction(&testTransaction)
//	timeout, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
//	defer cancel()
//	// For this test, we will return Pending status forever, so we need it to time out
//	// in order to capture and verify that pending status.
//	newTransaction, err = Drive(timeout, &bitflyerStateMachine)
//	// The only tolerable error is a timeout, and that's what we expect here
//	must.True(t, errors.Is(err, context.DeadlineExceeded))
//	should.Equal(t, paymentLib.Pending, newTransaction.Status)
//
//	// Should transition transaction into the Paid state
//	// Mock transaction status check that will succeed, overriding the one about that will stay
//	// stuck in pending
//	jsonCheckStatusResponse, err := json.Marshal(bitflyerTransactionCheckStatusSuccessResponse)
//	must.Equal(t, nil, err)
//	httpmock.RegisterResponder(
//		"POST",
//		fmt.Sprintf(
//			"%s/api/link/v1/coin/withdraw-to-deposit-id/bulk-status",
//			mockBitflyerHost,
//		),
//		httpmock.NewStringResponder(200, string(jsonCheckStatusResponse)),
//	)
//	testTransaction.Status = paymentLib.Pending
//	marshaledData, _ = json.Marshal(testTransaction)
//	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
//	bitflyerStateMachine.setTransaction(&testTransaction)
//	// This test shouldn't time out, but if it gets stuck in pending the defaul Drive timeout
//	// is 5 minutes and we don't want the test to run that long even if it's broken.
//	timeout, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
//	defer cancel()
//	newTransaction, err = Drive(timeout, &bitflyerStateMachine)
//	must.Equal(t, nil, err)
//	should.Equal(t, paymentLib.Paid, newTransaction.Status)
//}
//
//// TestBitflyerStateMachine500FailureToPaidTransition tests for a failure to progress status
//// after a 500 error response while attempting to transfer from Pending to Paid
//func TestBitflyerStateMachine500FailureToPaidTransition(t *testing.T) {
//	httpmock.Activate()
//	defer httpmock.DeactivateAndReset()
//
//	// Mock transaction commit that will fail
//	jsonResponse, err := json.Marshal(bitflyerTransactionSubmitFailureResponse)
//	must.Equal(t, nil, err)
//	httpmock.RegisterResponder(
//		"POST",
//		fmt.Sprintf(
//			"%s/api/link/v1/coin/withdraw-to-deposit-id/bulk-status",
//			mockBitflyerHost,
//		),
//		httpmock.NewStringResponder(500, string(jsonResponse)),
//	)
//
//	ctx := context.Background()
//	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
//	//mockDriver := new(mockDriver)
//	//service := Service{
//	//datastore: mockDriver,
//	//baseCtx: context.Background(),
//	//}
//	id := ""
//	authorizations := []paymentLib.PaymentAuthorization{
//		{
//			KeyID:      "1",
//			DocumentID: "A",
//		},
//		{
//			KeyID:      "2",
//			DocumentID: "A",
//		},
//	}
//	transaction := paymentLib.AuthenticatedPaymentState{Status: paymentLib.Prepared, DocumentID: id, Authorizations: authorizations}
//	bitflyerStateMachine := BitflyerMachine{}
//	//bitflyerStateMachine.setPersistenceConfigValues(
//	//service.datastore,
//	//service.sdkClient,
//	//service.kmsSigningClient,
//	//service.kmsSigningKeyID,
//	bitflyerStateMachine.setTransaction(
//		&transaction,
//	)
//	// When the implementation is in place, this Version value will not be necessary.
//	// However, it's set here to allow the placeholder implementation to return the
//	// correct value and allow this test to pass in the meantime.
//	// @TODO: Make this test fail
//	// currentVersion := 500
//
//	newState, err := Drive(ctx, &bitflyerStateMachine)
//	must.Nil(t, err)
//	should.Equal(t, paymentLib.Authorized, newState.Status)
//}
//
//// TestBitflyerStateMachine404FailureToPaidTransition tests for a failure to progress status
//// Failure with 404 error when attempting to transfer from Pending to Paid
//func TestBitflyerStateMachine404FailureToPaidTransition(t *testing.T) {
//	err := os.Setenv("BITFLYER_ENVIRONMENT", "test")
//	must.Equal(t, nil, err)
//	err = os.Setenv("BITFLYER_SERVER", mockBitflyerHost)
//	must.Equal(t, nil, err)
//	httpmock.Activate()
//	defer httpmock.DeactivateAndReset()
//
//	// Mock transaction commit that will fail
//	jsonResponse, err := json.Marshal(bitflyerTransactionCheckStatusFailureResponse)
//	must.Equal(t, nil, err)
//	httpmock.RegisterResponder(
//		"POST",
//		fmt.Sprintf(
//			"%s/api/link/v1/coin/withdraw-to-deposit-id/bulk-status",
//			mockBitflyerHost,
//		),
//		httpmock.NewStringResponder(404, string(jsonResponse)),
//	)
//	jsonTokenRefreshResponse, err := json.Marshal(bitflyerTransactionTokenRefreshResponse)
//	must.Equal(t, nil, err)
//	httpmock.RegisterResponder(
//		"POST",
//		fmt.Sprintf(
//			"%s/api/link/v1/token",
//			mockBitflyerHost,
//		),
//		httpmock.NewStringResponder(200, string(jsonTokenRefreshResponse)),
//	)
//
//	ctx := context.Background()
//	//mockDriver := new(mockDriver)
//	//service := Service{
//	//datastore: mockDriver,
//	//baseCtx: context.Background(),
//	//}
//	id := ""
//	transaction := paymentLib.AuthenticatedPaymentState{Status: paymentLib.Pending, DocumentID: id}
//	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
//	bitflyerStateMachine := BitflyerMachine{}
//	//bitflyerStateMachine.setPersistenceConfigValues(
//	//service.datastore,
//	//service.sdkClient,
//	//service.kmsSigningClient,
//	//service.kmsSigningKeyID,
//	bitflyerStateMachine.setTransaction(
//		&transaction,
//	)
//	// When the implementation is in place, this Version value will not be necessary.
//	// However, it's set here to allow the placeholder implementation to return the
//	// correct value and allow this test to pass in the meantime.
//	// @TODO: Make this test fail
//	// currentVersion := 404
//
//	newState, err := Drive(ctx, &bitflyerStateMachine)
//	must.Nil(t, err)
//	should.Equal(t, paymentLib.Pending, newState.Status)
//}

package payments

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"github.com/brave-intl/bat-go/libs/clients/zebpay"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	"github.com/jarcoal/httpmock"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

var (
	mockZebpayHost = "http://fakebravesoftware.com"
)

type ctxAuthKey struct{}

/*
TestMockedZebpayStateMachineHappyPathTransitions tests for correct state progression from
Initialized to Paid. Additionally, Paid status should be final and Failed status should
be permanent.
*/
func TestMockedZebpayStateMachineHappyPathTransitions(t *testing.T) {
	err := os.Setenv("ZEBPAY_ENVIRONMENT", "test")
	must.Nil(t, err)
	err = os.Setenv("ZEBPAY_SERVER", mockZebpayHost)
	must.Nil(t, err)

	zebpayClient, err := zebpay.NewWithHTTPClient(http.Client{})
	must.Nil(t, err)

	zebpayStateMachine := ZebpayMachine{
		client:     zebpayClient,
		zebpayHost: mockZebpayHost,
	}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	// Mock transaction creation that will succeed
	zebpaySumbitSuccessResponse, err := json.Marshal(zebpayTransactionSubmitSuccessResponse)
	must.Nil(t, err)
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/api/bulktransfer",
			mockZebpayHost,
		),
		httpmock.NewStringResponder(200, string(zebpaySumbitSuccessResponse)),
	)

	// Mock transaction status check that will stay stuck in pending
	zebpayCheckStatusResponsePending, err := json.Marshal(
		zebpayTransactionCheckStatusSuccessResponsePending,
	)
	must.Nil(t, err)
	httpmock.RegisterResponder(
		"GET",
		fmt.Sprintf(
			"%s/api/checktransferstatus/725c920b-d158-56fb-b5cf-5910d9ca4a16/status",
			mockZebpayHost,
		),
		httpmock.NewStringResponder(200, string(zebpayCheckStatusResponsePending)),
	)

	idempotencyKey, err := uuid.Parse("1803df27-f29c-537a-9384-bb5b523ea3f7")
	must.Nil(t, err)

	testState := paymentLib.AuthenticatedPaymentState{
		Status: paymentLib.Prepared,
		PaymentDetails: paymentLib.PaymentDetails{
			Amount:    decimal.NewFromFloat(13.736461457342187),
			To:        "512",
			From:      "c6911095-ba83-4aa1-b0fb-15934568a65a",
			Custodian: "zebpay",
			PayoutID:  "123456",
			Currency:  "BAT",
		},
		Authorizations: []paymentLib.PaymentAuthorization{{}, {}, {}},
	}

	marshaledData, _ := json.Marshal(testState)
	must.Nil(t, err)
	privkey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must.Nil(t, err)
	marshalledPubkey, err := x509.MarshalPKIXPublicKey(&privkey.PublicKey)
	must.Nil(t, err)
	mockTransitionHistory := QLDBPaymentTransitionHistoryEntry{
		BlockAddress: QLDBPaymentTransitionHistoryEntryBlockAddress{
			StrandID:   "test",
			SequenceNo: 1,
		},
		Hash: []byte("test"),
		Data: paymentLib.PaymentState{
			UnsafePaymentState: marshaledData,
			Signature:          []byte{},
			ID:                 idempotencyKey,
			PublicKey:          string(marshalledPubkey),
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
	mockKMS.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return(&kms.SignOutput{
		Signature: []byte("succeed"),
	}, nil)
	mockKMS.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(&kms.VerifyOutput{
		SignatureValid: true,
	}, nil)
	mockKMS.On("GetPublicKey", mock.Anything, mock.Anything, mock.Anything).Return(
		&kms.GetPublicKeyOutput{
			PublicKey: marshalledPubkey,
		},
		nil,
	)

	zebpayStateMachine.setTransaction(
		&testState,
	)

	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")

	// First call in order is to insertPayment and should return a fake document ID
	mockDriver.On("Execute", mock.Anything, mock.Anything).Return("123456", nil).Once()
	// Next call in order is to get GetTransactionFromDocumentID and should return an
	// AuthenticatedPaymentState.
	mockDriver.On("Execute", mock.Anything, mock.Anything).Return(&testState, nil).Once()
	// All further calls should return the mocked history entry.
	mockDriver.On("Execute", mock.Anything, mock.Anything).Return(&testState, nil)

	// Should transition transaction into the Authorized state
	testState.Status = paymentLib.Prepared
	marshaledData, _ = json.Marshal(testState)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	zebpayStateMachine.setTransaction(&testState)
	newTransaction, err := Drive(ctx, &zebpayStateMachine)
	must.Nil(t, err)
	// Ensure that our Bitflyer calls are going through the mock and not anything else.
	//must.Equal(t, info[tokenInfoKey], 1)
	should.Equal(t, paymentLib.Authorized, newTransaction.Status)

	// Should transition transaction into the Pending state
	testState.Status = paymentLib.Authorized
	marshaledData, _ = json.Marshal(testState)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	zebpayStateMachine.setTransaction(&testState)
	timeout, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()
	// For this test, we will return Pending status forever, so we need it to time out
	// in order to capture and verify that pending status.
	newTransaction, err = Drive(timeout, &zebpayStateMachine)
	// The only tolerable error is a timeout, and that's what we expect here
	must.ErrorIs(t, err, context.DeadlineExceeded)
	should.Equal(t, paymentLib.Pending, newTransaction.Status)

	// Should transition transaction into the Paid state
	// Mock transaction status check that will succeed, overriding the one above that will stay
	// stuck in pending
	zebpayCheckStatusResponseSuccess, err := json.Marshal(
		zebpayTransactionCheckStatusSuccessResponse,
	)
	must.Nil(t, err)
	httpmock.RegisterResponder(
		"GET",
		fmt.Sprintf(
			"%s/api/checktransferstatus/725c920b-d158-56fb-b5cf-5910d9ca4a16/status",
			mockZebpayHost,
		),
		httpmock.NewStringResponder(200, string(zebpayCheckStatusResponseSuccess)),
	)
	testState.Status = paymentLib.Pending
	marshaledData, _ = json.Marshal(testState)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	zebpayStateMachine.setTransaction(&testState)
	// This test shouldn't time out, but if it gets stuck in pending the defaul Drive timeout
	// is 5 minutes and we don't want the test to run that long even if it's broken.
	timeout, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	newTransaction, err = Drive(timeout, &zebpayStateMachine)
	must.Equal(t, nil, err)
	should.Equal(t, paymentLib.Paid, newTransaction.Status)
}

// TestMockedZebpayStateMachineAuthorizedToPendingTransition tests the progression from Prepared to
// Authorized when sufficient authorizers are present. When an authorization is missing it also
// tests waiting for a new authorization.
func TestMockedZebpayStateMachineAuthorizedToPendingTransition(t *testing.T) {
	zebpayClient, err := zebpay.NewWithHTTPClient(http.Client{})
	must.Nil(t, err)

	zebpayStateMachine := ZebpayMachine{
		client:     zebpayClient,
		zebpayHost: mockZebpayHost,
	}
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	// Mock transaction commit that will fail
	jsonResponse, err := json.Marshal(zebpayTransactionSubmitFailureResponse)
	must.Equal(t, nil, err)
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/api/bulktransfer",
			mockZebpayHost,
		),
		httpmock.NewStringResponder(200, string(jsonResponse)),
	)

	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	id := ""
	transaction := paymentLib.AuthenticatedPaymentState{
		Status:     paymentLib.Prepared,
		DocumentID: id,
		Authorizations: []paymentLib.PaymentAuthorization{
			{
				KeyID:      "1",
				DocumentID: "A",
			},
		},
	}
	zebpayStateMachine.setTransaction(
		&transaction,
	)

	newState, err := Drive(ctx, &zebpayStateMachine)
	must.ErrorIs(t, err, &InsufficientAuthorizationsError{})
	should.Equal(t, paymentLib.Prepared, newState.Status)
	transaction.Authorizations = append(transaction.Authorizations,
		paymentLib.PaymentAuthorization{
			KeyID:      "2",
			DocumentID: "A",
		},
	)
	newState, err = Drive(ctx, &zebpayStateMachine)
	must.Nil(t, err)
	should.Equal(t, paymentLib.Authorized, newState.Status)
}

// TestMockedZebpayStateMachine500FailureToPendingTransition tests for a failure to progress status
// after a 500 error response while attempting to transfer from Pending to Paid
func TestMockedZebpayStateMachine500FailureToPendingTransition(t *testing.T) {
	zebpayClient, err := zebpay.NewWithHTTPClient(http.Client{})
	must.Nil(t, err)

	zebpayStateMachine := ZebpayMachine{
		client:     zebpayClient,
		zebpayHost: mockZebpayHost,
	}
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	// Mock transaction commit that will fail
	jsonResponse, err := json.Marshal(zebpayTransactionSubmitFailureResponse)
	must.Equal(t, nil, err)
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/api/bulktransfer",
			mockZebpayHost,
		),
		httpmock.NewStringResponder(500, string(jsonResponse)),
	)

	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	id := ""
	transaction := paymentLib.AuthenticatedPaymentState{
		Status:     paymentLib.Authorized,
		DocumentID: id,
		PaymentDetails: paymentLib.PaymentDetails{
			Amount:    decimal.NewFromFloat(13.736461457342187),
			To:        "512",
			From:      "c6911095-ba83-4aa1-b0fb-15934568a65a",
			Custodian: "zebpay",
			PayoutID:  "123456",
			Currency:  "BAT",
		},
	}
	zebpayStateMachine.setTransaction(
		&transaction,
	)

	newState, err := Drive(ctx, &zebpayStateMachine)
	should.NotNil(t, err)
	must.Equal(t, paymentLib.Authorized, newState.Status)
}

// TestMockedZebpayStateMachine500FailureToPaidTransition tests for a failure to progress status
// after a 500 error response while attempting to transfer from Pending to Paid
func TestMockedZebpayStateMachine500FailureToPaidTransition(t *testing.T) {
	zebpayClient, err := zebpay.NewWithHTTPClient(http.Client{})
	must.Nil(t, err)

	zebpayStateMachine := ZebpayMachine{
		client:     zebpayClient,
		zebpayHost: mockZebpayHost,
	}
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	// Mock transaction status check that will stay stuck in pending
	zebpayCheckStatusResponsePending, err := json.Marshal(
		zebpayTransactionCheckStatusSuccessResponsePending,
	)
	must.Nil(t, err)
	httpmock.RegisterResponder(
		"GET",
		fmt.Sprintf(
			"%s/api/checktransferstatus/725c920b-d158-56fb-b5cf-5910d9ca4a16/status",
			mockZebpayHost,
		),
		httpmock.NewStringResponder(404, string(zebpayCheckStatusResponsePending)),
	)

	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	id := ""
	transaction := paymentLib.AuthenticatedPaymentState{
		Status:     paymentLib.Authorized,
		DocumentID: id,
		PaymentDetails: paymentLib.PaymentDetails{
			Amount:    decimal.NewFromFloat(13.736461457342187),
			To:        "512",
			From:      "c6911095-ba83-4aa1-b0fb-15934568a65a",
			Custodian: "zebpay",
			PayoutID:  "123456",
			Currency:  "BAT",
		},
	}
	zebpayStateMachine.setTransaction(
		&transaction,
	)

	newState, err := Drive(ctx, &zebpayStateMachine)
	must.NotNil(t, err)
	must.Equal(t, paymentLib.Authorized, newState.Status)
}

// TestMockedZebpayStateMachine404FailureToPendingTransition tests for a failure to progress status
// Failure with 404 error when attempting to transfer from Pending to Paid
func TestMockedZebpayStateMachine404FailureToPendingTransition(t *testing.T) {
	zebpayClient, err := zebpay.NewWithHTTPClient(http.Client{})
	must.Nil(t, err)

	zebpayStateMachine := ZebpayMachine{
		client:     zebpayClient,
		zebpayHost: mockZebpayHost,
	}
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	// Mock transaction commit that will fail
	jsonResponse, err := json.Marshal(zebpayTransactionSubmitFailureResponse)
	must.Equal(t, nil, err)
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/api/bulktransfer",
			mockZebpayHost,
		),
		httpmock.NewStringResponder(404, string(jsonResponse)),
	)

	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	id := ""
	transaction := paymentLib.AuthenticatedPaymentState{
		Status:     paymentLib.Authorized,
		DocumentID: id,
		PaymentDetails: paymentLib.PaymentDetails{
			Amount:    decimal.NewFromFloat(13.736461457342187),
			To:        "512",
			From:      "c6911095-ba83-4aa1-b0fb-15934568a65a",
			Custodian: "zebpay",
			PayoutID:  "123456",
			Currency:  "BAT",
		},
	}
	zebpayStateMachine.setTransaction(
		&transaction,
	)

	newState, err := Drive(ctx, &zebpayStateMachine)
	must.NotNil(t, err)
	must.Equal(t, paymentLib.Authorized, newState.Status)
}

// TestMockedZebpayStateMachine404FailureToPaidTransition tests for a failure to progress status
// Failure with 404 error when attempting to transfer from Pending to Paid
func TestMockedZebpayStateMachine404FailureToPaidTransition(t *testing.T) {
	zebpayClient, err := zebpay.NewWithHTTPClient(http.Client{})
	must.Nil(t, err)

	zebpayStateMachine := ZebpayMachine{
		client:     zebpayClient,
		zebpayHost: mockZebpayHost,
	}
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	// Mock transaction status check that will stay stuck in pending
	zebpayCheckStatusResponsePending, err := json.Marshal(
		zebpayTransactionCheckStatusSuccessResponsePending,
	)
	must.Nil(t, err)
	httpmock.RegisterResponder(
		"GET",
		fmt.Sprintf(
			"%s/api/checktransferstatus/725c920b-d158-56fb-b5cf-5910d9ca4a16/status",
			mockZebpayHost,
		),
		httpmock.NewStringResponder(404, string(zebpayCheckStatusResponsePending)),
	)

	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	id := ""
	transaction := paymentLib.AuthenticatedPaymentState{
		Status:     paymentLib.Authorized,
		DocumentID: id,
		PaymentDetails: paymentLib.PaymentDetails{
			Amount:    decimal.NewFromFloat(13.736461457342187),
			To:        "512",
			From:      "c6911095-ba83-4aa1-b0fb-15934568a65a",
			Custodian: "zebpay",
			PayoutID:  "123456",
			Currency:  "BAT",
		},
	}
	zebpayStateMachine.setTransaction(
		&transaction,
	)

	newState, err := Drive(ctx, &zebpayStateMachine)
	must.NotNil(t, err)
	must.Equal(t, paymentLib.Authorized, newState.Status)

	transaction.Status = paymentLib.Pending
	newState, err = Drive(ctx, &zebpayStateMachine)
	must.NotNil(t, err)
	must.Equal(t, paymentLib.Pending, newState.Status)
}

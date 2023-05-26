package payments

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	"github.com/jarcoal/httpmock"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

var (
	mockUpholdHost = "fake://mock.uphold.com"

	upholdWallet = uphold.Wallet{
		Info: walletutils.Info{
			ProviderID: "test",
			Provider:   "uphold",
			PublicKey:  "",
		},
		PrivKey: ed25519.PrivateKey([]byte("")),
		PubKey:  httpsignature.Ed25519PubKey([]byte("")),
	}
	upholdSucceedTransaction = custodian.Transaction{ProviderID: "1234"}
)

/*
TestUpholdStateMachineHappyPathTransitions tests for correct state progression from
Initialized to Paid. Additionally, Paid status should be final and Failed status should
be permanent.
*/
func TestUpholdStateMachineHappyPathTransitions(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	err := os.Setenv("UPHOLD_ENVIRONMENT", "test")
	must.Equal(t, nil, err)

	// Mock transaction creation
	jsonResponse, err := json.Marshal(upholdCreateTransactionSuccessResponse)
	must.Equal(t, nil, err)
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/v0/me/cards/%s/transactions",
			mockUpholdHost,
			upholdWallet.Info.ProviderID,
		),
		httpmock.NewStringResponder(200, string(jsonResponse)),
	)
	// Mock transaction commit that will succeed
	jsonResponse, err = json.Marshal(upholdCommitTransactionSuccessResponse)
	must.Equal(t, nil, err)
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/v0/me/cards/%s/transactions/%s/commit",
			mockUpholdHost,
			upholdWallet.Info.ProviderID,
			upholdSucceedTransaction.ProviderID,
		),
		httpmock.NewStringResponder(200, string(jsonResponse)),
	)

	namespaceUUID, err := uuid.Parse("7478bd8a-2247-493d-b419-368f1a1d7a6c")
	must.Equal(t, nil, err)
	idempotencyKey, err := uuid.Parse("f209929e-0ba9-5f56-a336-5b981bdaaaaf")
	must.Equal(t, nil, err)
	ctx := context.Background()
	ctx = context.WithValue(ctx, serviceNamespaceContextKey{}, namespaceUUID)
	upholdStateMachine := UpholdMachine{}

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
	upholdStateMachine.setService(&service)
	upholdStateMachine.setTransaction(&testTransaction)
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")

	// Should create a transaction in QLDB. Current state argument is empty because
	// the object does not yet exist.
	mockDriver.On("Execute", mock.Anything, mock.Anything).Return(nil, &QLDBReocrdNotFoundError{}).Once()
	mockDriver.On("Execute", mock.Anything, mock.Anything).Return(&mockTransitionHistory, nil)
	newTransaction, err := Drive(ctx, &upholdStateMachine)
	must.Equal(t, nil, err)
	should.Equal(t, Prepared, newTransaction.State)

	// Should transition transaction into the Authorized state
	testTransaction.State = Prepared
	marshaledData, _ = ion.MarshalBinary(testTransaction)
	mockTransitionHistory.Data.Data = marshaledData
	upholdStateMachine.setTransaction(&testTransaction)
	newTransaction, err = Drive(ctx, &upholdStateMachine)
	must.Equal(t, nil, err)
	should.Equal(t, Authorized, newTransaction.State)

	// Should transition transaction into the Pending state
	testTransaction.State = Authorized
	marshaledData, _ = ion.MarshalBinary(testTransaction)
	mockTransitionHistory.Data.Data = marshaledData
	upholdStateMachine.setTransaction(&testTransaction)
	newTransaction, err = Drive(ctx, &upholdStateMachine)
	must.Equal(t, nil, err)
	should.Equal(t, Pending, newTransaction.State)

	// Should transition transaction into the Paid state
	testTransaction.State = Pending
	marshaledData, _ = ion.MarshalBinary(testTransaction)
	mockTransitionHistory.Data.Data = marshaledData
	upholdStateMachine.setTransaction(&testTransaction)
	newTransaction, err = Drive(ctx, &upholdStateMachine)
	must.Equal(t, nil, err)
	should.Equal(t, Paid, newTransaction.State)
}

/*
TestUpholdStateMachine500FailureToPendingTransitions tests for a failure to progress status
after a 500 error response while attempting to transfer from Pending to Paid
func TestUpholdStateMachine500FailureToPendingTransitions(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	err := os.Setenv("UPHOLD_ENVIRONMENT", "test")
	must.Equal(t, nil, err)

	// Mock transaction creation that will fail
	jsonResponse, err := json.Marshal(upholdCreateTransactionFailureResponse)
	must.Equal(t, nil, err)
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/v0/me/cards/%s/transactions",
			mockUpholdHost,
			upholdWallet.Info.ProviderID,
		),
		httpmock.NewStringResponder(500, string(jsonResponse)),
	)

	ctx := context.Background()
	mockDriver := new(mockDriver)
	id := uuid.New()
	transaction := Transaction{State: Authorized, ID: &id}
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	upholdStateMachine := UpholdMachine{}
	service := Service{
		datastore: mockDriver,
		baseCtx:   context.Background(),
	}
	upholdStateMachine.setService(&service)
	upholdStateMachine.setTransaction(&transaction)
	// When the implementation is in place, this Version value will not be necessary.
	// However, it's set here to allow the placeholder implementation to return the
	// correct value and allow this test to pass in the meantime.
	// @TODO: Make this test fail
	// currentVersion := 500

	// Should fail to transition transaction into the Pending state
	newState, _ := Drive(ctx, &upholdStateMachine)
	should.Equal(t, Authorized, newState)
}
*/

/*
TestUpholdStateMachine404FailureToPaidTransitions tests for a failure to progress status
Failure with 404 error when attempting to transfer from Pending to Paid
func TestUpholdStateMachine404FailureToPaidTransitions(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	err := os.Setenv("UPHOLD_ENVIRONMENT", "test")
	must.Equal(t, nil, err)

	// Mock transaction commit that will fail
	jsonResponse, err := json.Marshal(upholdCommitTransactionFailureResponse)
	must.Equal(t, nil, err)
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/v0/me/cards/%s/transactions/%s/commit",
			mockUpholdHost,
			upholdWallet.Info.ProviderID,
			geminiSucceedTransaction.ProviderID,
		),
		httpmock.NewStringResponder(404, string(jsonResponse)),
	)

	ctx := context.Background()
	id := uuid.New()
	transaction := Transaction{State: Pending, ID: &id}
	mockDriver := new(mockDriver)
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	// When the implementation is in place, this Version value will not be necessary.
	// However, it's set here to allow the placeholder implementation to return the
	// correct value and allow this test to pass in the mean time.
	// @TODO: Make this test fail
	// currentVersion := 404
	upholdStateMachine := UpholdMachine{}
	service := Service{
		datastore: mockDriver,
		baseCtx:   context.Background(),
	}
	upholdStateMachine.setService(&service)
	upholdStateMachine.setTransaction(&transaction)

	// Should transition transaction into the Pending state
	newState, _ := Drive(ctx, &upholdStateMachine)
	should.Equal(t, Pending, newState)
}
*/

package payments

import (
	"crypto/ed25519"

	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
)

var (
	// TODO: unused
	//mockUpholdHost = "fake://mock.uphold.com"

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
/*
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
	idempotencyKey, err := uuid.Parse("efc57510-def0-5667-a64b-eead7abc1a4f")
	must.Equal(t, nil, err)
	ctx := context.Background()
	ctx = context.WithValue(ctx, serviceNamespaceContextKey{}, namespaceUUID)
	upholdStateMachine := UpholdMachine{}

	testTransaction := AuthenticatedPaymentState{
		Status: Prepared,
		PaymentDetails: PaymentDetails{
			Amount:    decimal.NewFromFloat(1.1),
			Custodian: "uphold",
		},
		Authorizations: []PaymentAuthorization{{}, {}, {}},
	}
	marshaledData, err := json.Marshal(testTransaction)
	must.Equal(t, nil, err)
	mockTransitionHistory := QLDBPaymentTransitionHistoryEntry{
		BlockAddress: QLDBPaymentTransitionHistoryEntryBlockAddress{
			StrandID:   "test",
			SequenceNo: 1,
		},
		Hash: []byte("test"),
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
		//datastore:        mockDriver,
		kmsSigningClient: mockKMS,
		baseCtx:          context.Background(),
	}
	//upholdStateMachine.setPersistenceConfigValues(
	//service.datastore,
	//service.sdkClient,
	//service.kmsSigningClient,
	//service.kmsSigningKeyID,
	upholdStateMachine.setTransaction(
		&testTransaction,
	)
	upholdStateMachine.setTransaction(&testTransaction)
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
	//	newTransaction, _, err := service.GetTransactionFromDocumentID(ctx, insertedDocumentID)
	//	must.Equal(t, nil, err)
	//	should.Equal(t, Prepared, newTransaction.Status)

	// Should transition transaction into the Authorized state
	//	testTransaction.Status = Prepared
	//	marshaledData, _ = json.Marshal(testTransaction)
	//	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	//	upholdStateMachine.setTransaction(&testTransaction)
	//	newTransaction, err = Drive(ctx, &upholdStateMachine)
	//	must.Equal(t, nil, err)
	//	should.Equal(t, Authorized, newTransaction.Status)

	//	// Should transition transaction into the Pending state
	//	testTransaction.Status = Authorized
	//	marshaledData, _ = json.Marshal(testTransaction)
	//	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	//	upholdStateMachine.setTransaction(&testTransaction)
	//	newTransaction, err = Drive(ctx, &upholdStateMachine)
	//	must.Equal(t, nil, err)
	//	// @TODO: When tests include custodial mocks, this should be Pending
	//	should.Equal(t, Paid, newTransaction.Status)
	//
	//	// Should transition transaction into the Paid state
	//	testTransaction.Status = Pending
	//	marshaledData, _ = json.Marshal(testTransaction)
	//	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	//	upholdStateMachine.setTransaction(&testTransaction)
	//	newTransaction, err = Drive(ctx, &upholdStateMachine)
	//	must.Equal(t, nil, err)
	//	should.Equal(t, Paid, newTransaction.Status)
}
*/

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

package payments

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/google/uuid"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/service/qldb"
	qldbTypes "github.com/aws/aws-sdk-go-v2/service/qldb/types"
	"github.com/aws/smithy-go/middleware"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
	should "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	must "github.com/stretchr/testify/require"
)

// Unit testing the package code using a Mock Driver
type mockDriver struct {
	mock.Mock
}

// Unit testing the package code using a Mock QLDB SDK
type mockSDKClient struct {
	mock.Mock
}

type mockResult struct {
	mock.Mock
}

type mockKMSClient struct {
	mock.Mock
}

func (m *mockDriver) Execute(ctx context.Context, fn func(txn qldbdriver.Transaction) (interface{}, error)) (interface{}, error) {
	args := m.Called(ctx, fn)
	return args.Get(0), args.Error(1)
}

func (m *mockDriver) Shutdown(ctx context.Context) {
	return
}

func (m *mockResult) GetCurrentData() []byte {
	args := m.Called()
	return args.Get(0).([]byte)
}
func (m *mockResult) Next(txn wrappedQldbTxnAPI) bool {
	args := m.Called(txn)
	return args.Get(0).(bool)
}

func (m *mockSDKClient) New() *wrappedQldbSdkClient {
	args := m.Called()
	return args.Get(0).(*wrappedQldbSdkClient)
}
func (m *mockSDKClient) GetDigest(
	ctx context.Context,
	params *qldb.GetDigestInput,
	optFns ...func(*qldb.Options),
) (*qldb.GetDigestOutput, error) {
	args := m.Called()
	return args.Get(0).(*qldb.GetDigestOutput), args.Error(1)
}

func (m *mockSDKClient) GetRevision(
	ctx context.Context,
	params *qldb.GetRevisionInput,
	optFns ...func(*qldb.Options),
) (*qldb.GetRevisionOutput, error) {
	args := m.Called()
	return args.Get(0).(*qldb.GetRevisionOutput), args.Error(1)
}

func (m *mockKMSClient) Sign(ctx context.Context, params *kms.SignInput, optFns ...func(*kms.Options)) (*kms.SignOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*kms.SignOutput), args.Error(1)
}

func (m *mockKMSClient) Verify(ctx context.Context, params *kms.VerifyInput, optFns ...func(*kms.Options)) (*kms.VerifyOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*kms.VerifyOutput), args.Error(1)
}

func (m *mockKMSClient) GetPublicKey(ctx context.Context, params *kms.GetPublicKeyInput, optFns ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*kms.GetPublicKeyOutput), args.Error(1)
}

/*
Traverse QLDB history for a transaction and ensure that only valid transitions have occurred.
Should include exhaustive passing and failing tests.
*/
func TestVerifyPaymentTransitionHistory(t *testing.T) {
	namespaceUUID, err := uuid.Parse("7478bd8a-2247-493d-b419-368f1a1d7a6c")
	must.Equal(t, nil, err)
	idempotencyKey, err := uuid.Parse("48f64a63-cb9f-5297-a605-964637448b9f")
	must.Equal(t, nil, err)
	testData := Transaction{
		State: Prepared,
	}
	marshaledData, err := ion.MarshalBinary(testData)
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
	binaryTransitionHistory, err := ion.MarshalBinary(mockTransitionHistory)
	must.Equal(t, nil, err)
	mockKMS := new(mockKMSClient)
	mockRes := new(mockResult)
	mockRes.On("GetCurrentData").Return(binaryTransitionHistory)
	mockDriver := new(mockDriver)
	mockDriver.On("Execute", mock.Anything, mock.Anything).Return(&mockTransitionHistory, nil)
	mockKMS.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return(&kms.SignOutput{Signature: []byte("succeed")}, nil)
	mockKMS.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(&kms.VerifyOutput{SignatureValid: true}, nil)
	mockKMS.On("GetPublicKey", mock.Anything, mock.Anything, mock.Anything).Return(&kms.GetPublicKeyOutput{PublicKey: []byte("test")}, nil)

	ctx := context.Background()
	ctx = context.WithValue(ctx, serviceNamespaceContextKey{}, namespaceUUID)

	// Valid transitions should be valid
	for _, transactionHistorySet := range transactionHistorySetTrue {
		must.Equal(t, nil, err)
		valid, err := validateTransactionHistory(ctx, &idempotencyKey, namespaceUUID, transactionHistorySet, mockKMS)
		must.Equal(t, nil, err)
		should.True(t, valid)
	}
	mockKMS.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(&kms.VerifyOutput{SignatureValid: false}, nil)
	// Invalid transitions should be invalid
	for _, transactionHistorySet := range transactionHistorySetFalse {
		valid, _ := validateTransactionHistory(ctx, &idempotencyKey, namespaceUUID, transactionHistorySet, mockKMS)
		should.False(t, valid)
	}
}

// Test that QLDB revisions are valid by generating a digest from a set of hashes.
func TestValidateRevision(t *testing.T) {
	/*
		Hashes in below true object were calculated like so:
			hash1 := sha256.Sum256([]byte{1})
			hash2 := sha256.Sum256([]byte{2})
			hash3 := sha256.Sum256([]byte{3})
			hash4 := sha256.Sum256([]byte{4})
			concatenated21 := append(hash2[:], hash1[:]...)
			hash12 := sha256.Sum256(concatenated21)
			concatenated34 := append(hash4[:], hash3[:]...)
			hash34 := sha256.Sum256(concatenated34)
			concatenatedDigest := append(hash34[:], hash12[:]...)
			hashDigest := sha256.Sum256(concatenatedDigest)
	*/

	var (
		mockSDKClient = new(mockSDKClient)
		trueObject    = qldbPaymentTransitionHistoryEntry{
			BlockAddress: qldbPaymentTransitionHistoryEntryBlockAddress{
				StrandID:   "strand1",
				SequenceNo: 10,
			},
			Hash: "28G0yQD/5I1XW12lxjgEASX2XbD+PiRJS3bqmGRX2YY=",
			Data: qldbPaymentTransitionHistoryEntryData{
				Signature: []byte{},
				Data:      []byte{},
			},
			Metadata: qldbPaymentTransitionHistoryEntryMetadata{
				ID:      "transitionid1",
				Version: 10,
				TxTime:  time.Now(),
				TxID:    "",
			},
		}
		falseObject = qldbPaymentTransitionHistoryEntry{
			BlockAddress: qldbPaymentTransitionHistoryEntryBlockAddress{
				StrandID:   "strand2",
				SequenceNo: 10,
			},
			Hash: "dGVzdGVzdGVzdAo=",
			Data: qldbPaymentTransitionHistoryEntryData{
				Signature: []byte{},
				Data:      []byte{},
			},
			Metadata: qldbPaymentTransitionHistoryEntryMetadata{
				ID:      "transitionid2",
				Version: 10,
				TxTime:  time.Now(),
				TxID:    "",
			},
		}
	)
	ctx := context.Background()
	tipAddress := "1234"
	revision := "revision data"
	testDigest := "JotSZH8zgqzUSDG+yH1m5IetvWVZlS7q+g0H33FuupY="
	testProofIonText := "[{{S/USLzRFVMU73i67jNK349FgCtYxw4Wl18ziPHeFRZo=}},{{fBBxpm1CZxVROypAOCfEGug6Gwg+zFOq2WFSGuHdt1w=}}]"

	testDigestOutput := qldb.GetDigestOutput{
		Digest:           []byte(testDigest),
		DigestTipAddress: &qldbTypes.ValueHolder{IonText: &tipAddress},
		ResultMetadata:   middleware.Metadata{},
	}
	testRevisionOutput := qldb.GetRevisionOutput{
		Revision:       &qldbTypes.ValueHolder{IonText: &revision},
		Proof:          &qldbTypes.ValueHolder{IonText: &testProofIonText},
		ResultMetadata: middleware.Metadata{},
	}
	mockSDKClient.On("GetDigest").Return(&testDigestOutput, nil)
	mockSDKClient.On("GetRevision").Return(&testRevisionOutput, nil)
	valid, err := revisionValidInTree(ctx, mockSDKClient, &trueObject)
	must.Equal(t, nil, err)
	should.True(t, valid)

	valid, err = revisionValidInTree(ctx, mockSDKClient, &falseObject)
	must.Equal(t, nil, err)
	should.False(t, valid)
}

/*
Generate all valid transition sequences and ensure that this test contains the exact same set of
valid transition sequences. The purpose of this test is to alert us if outside changes
impact the set of valid transitions.
*/
func TestGenerateAllValidTransitions(t *testing.T) {
	allValidTransitionSequences := GetAllValidTransitionSequences()
	knownValidTransitionSequences := [][]TransactionState{
		{Prepared, Authorized, Pending, Paid},
		{Prepared, Authorized, Pending, Failed},
		{Prepared, Authorized, Failed},
		{Prepared, Failed},
	}
	// Ensure all generatedTransitionSequence have a matching knownValidTransitionSequences
	for _, generatedTransitionSequence := range allValidTransitionSequences {
		foundMatch := false
		for _, knownValidTransitionSequence := range knownValidTransitionSequences {
			if reflect.DeepEqual(generatedTransitionSequence, knownValidTransitionSequence) {
				foundMatch = true
			}
		}
		should.True(t, foundMatch)
	}
	// Ensure all knownValidTransitionSequences have a matching generatedTransitionSequence
	for _, knownValidTransitionSequence := range allValidTransitionSequences {
		foundMatch := false
		for _, generatedTransitionSequence := range allValidTransitionSequences {
			if reflect.DeepEqual(generatedTransitionSequence, knownValidTransitionSequence) {
				foundMatch = true
			}
		}
		should.True(t, foundMatch)
	}
}

// TestQLDBSignedInteractions mocks QLDB to test signing and verifying of records that are
// persisted into QLDB
func TestQLDBSignedInteractions(t *testing.T) {
	testData := Transaction{
		State: Prepared,
	}
	marshaledData, err := ion.MarshalBinary(testData)
	must.Equal(t, nil, err)
	mockTransitionHistory := qldbPaymentTransitionHistoryEntry{
		BlockAddress: qldbPaymentTransitionHistoryEntryBlockAddress{
			StrandID:   "test",
			SequenceNo: 1,
		},
		Hash: "test",
		Data: qldbPaymentTransitionHistoryEntryData{
			Data:      marshaledData,
			Signature: []byte{},
		},
		Metadata: qldbPaymentTransitionHistoryEntryMetadata{
			ID:      "test",
			Version: 1,
			TxTime:  time.Now(),
			TxID:    "test",
		},
	}
	binaryTransitionHistory, err := ion.MarshalBinary(mockTransitionHistory)
	must.Equal(t, nil, err)
	ctx := context.Background()
	namespaceUUID, err := uuid.Parse("7478bd8a-2247-493d-b419-368f1a1d7a6c")
	must.Equal(t, nil, err)
	ctx = context.WithValue(ctx, serviceNamespaceContextKey{}, namespaceUUID)

	mockKMS := new(mockKMSClient)
	mockRes := new(mockResult)
	mockRes.On("GetCurrentData").Return(binaryTransitionHistory)
	mockDriver := new(mockDriver)
	mockDriver.On("Execute", ctx, mock.Anything).Return(&mockTransitionHistory, nil)
	mockKMS.On("Sign", ctx, mock.Anything, mock.Anything).Return(&kms.SignOutput{Signature: []byte("succeed")}, nil)
	mockKMS.On("Verify", context.Background(), mock.Anything, mock.Anything).Return(&kms.VerifyOutput{SignatureValid: true}, nil).Once()
	mockKMS.On("Verify", context.Background(), mock.Anything, mock.Anything).Return(&kms.VerifyOutput{SignatureValid: false}, nil)
	mockKMS.On("GetPublicKey", context.Background(), mock.Anything, mock.Anything).Return(&kms.GetPublicKeyOutput{PublicKey: []byte("test")}, nil)

	todoString := ""
	message := []byte("test")
	signingOutput, _ := mockKMS.Sign(ctx, &kms.SignInput{
		KeyId:            &todoString,
		Message:          message,
		SigningAlgorithm: kmsTypes.SigningAlgorithmSpecEcdsaSha256,
	})
	mockTransitionHistory.Data.Signature = signingOutput.Signature
	service := Service{
		datastore:        mockDriver,
		baseCtx:          context.Background(),
		kmsSigningKeyID:  "123",
		kmsSigningClient: mockKMS,
	}

	// First write should succeed because Verify returns true
	_, err = service.WriteTransaction(ctx, &testData)
	should.NoError(t, err)
	// Second write of the same object should fail because Verify returns false
	// _, err := WriteTransaction(ctx, mockDriver, nil, mockKMS, &testData)
	// should.Error(t, err)
}

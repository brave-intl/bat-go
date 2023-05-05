package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"reflect"
	"testing"
	"time"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/service/qldb"
	qldbTypes "github.com/aws/aws-sdk-go-v2/service/qldb/types"
	"github.com/aws/smithy-go/middleware"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Unit testing the package code using a Mock Driver
type MockDriver struct {
	mock.Mock
}

// Unit testing the package code using a Mock QLDB SDK
type MockSDKClient struct {
	mock.Mock
}

type mockResult struct {
	mock.Mock
}

type mockTransaction struct {
	mock.Mock
}

type mockKMSClient struct {
	mock.Mock
}

func (m *mockTransaction) Execute(statement string, parameters ...interface{}) (wrappedQldbResult, error) {
	args := m.Called(statement, parameters)
	return args.Get(0).(wrappedQldbResult), args.Error(1)
}

func (m *mockTransaction) BufferResult(res *qldbdriver.Result) (*qldbdriver.BufferedResult, error) {
	panic("not used")
}

func (m *mockTransaction) Abort() error {
	panic("not used")
}

func (m *MockDriver) Execute(ctx context.Context, fn func(txn qldbdriver.Transaction) (interface{}, error)) (interface{}, error) {
	args := m.Called(ctx, fn)
	return args.Get(0).(*mockResult), args.Error(1)
}

func (m *MockDriver) Shutdown(ctx context.Context) {
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

func (m *MockSDKClient) New() *wrappedQldbSdkClient {
	args := m.Called()
	return args.Get(0).(*wrappedQldbSdkClient)
}
func (m *MockSDKClient) GetDigest(
	ctx context.Context,
	params *qldb.GetDigestInput,
	optFns ...func(*qldb.Options),
) (*qldb.GetDigestOutput, error) {
	args := m.Called()
	return args.Get(0).(*qldb.GetDigestOutput), args.Error(1)
}

func (m *MockSDKClient) GetRevision(
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

/*
Traverse QLDB history for a transaction and ensure that only valid transitions have occurred.
Should include exhaustive passing and failing tests.
*/
func TestVerifyPaymentTransitionHistory(t *testing.T) {
	// Valid transitions should be valid
	for _, transactionHistorySet := range transactionHistorySetTrue {
		valid, _ := validateTransitionHistory(transactionHistorySet)
		assert.True(t, valid)
	}
	// Invalid transitions should be invalid
	for _, transactionHistorySet := range transactionHistorySetFalse {
		valid, _ := validateTransitionHistory(transactionHistorySet)
		assert.False(t, valid)
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
		mockSDKClient = new(MockSDKClient)
		trueObject    = QLDBPaymentTransitionHistoryEntry{
			BlockAddress: qldbPaymentTransitionHistoryEntryBlockAddress{
				StrandID:   "strand1",
				SequenceNo: 10,
			},
			Hash: "28G0yQD/5I1XW12lxjgEASX2XbD+PiRJS3bqmGRX2YY=",
			Data: QLDBPaymentTransitionHistoryEntryData{
				Signature: []byte{},
				Data:      []byte{},
			},
			Metadata: QLDBPaymentTransitionHistoryEntryMetadata{
				ID:      "transitionid1",
				Version: 10,
				TxTime:  time.Now(),
				TxID:    "",
			},
		}
		falseObject = QLDBPaymentTransitionHistoryEntry{
			BlockAddress: qldbPaymentTransitionHistoryEntryBlockAddress{
				StrandID:   "strand2",
				SequenceNo: 10,
			},
			Hash: "dGVzdGVzdGVzdAo=",
			Data: QLDBPaymentTransitionHistoryEntryData{
				Signature: []byte{},
				Data:      []byte{},
			},
			Metadata: QLDBPaymentTransitionHistoryEntryMetadata{
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
	valid, err := RevisionValidInTree(ctx, mockSDKClient, trueObject)
	if err != nil {
		fmt.Printf("Failed true: %e", err)
	}
	assert.True(t, valid)

	valid, err = RevisionValidInTree(ctx, mockSDKClient, falseObject)
	if err != nil {
		fmt.Printf("Failed false: %e", err)
	}
	assert.False(t, valid)
}

/*
Generate all valid transition sequences and ensure that this test contains the exact same set of
valid transition sequences. The purpose of this test is to alert us if outside changes
impact the set of valid transitions.
*/
func TestGenerateAllValidTransitions(t *testing.T) {
	allValidTransitionSequences := GetAllValidTransitionSequences()
	knownValidTransitionSequences := [][]TransactionState{
		{0, 1, 2, 3, 4},
		{0, 1, 2, 3, 5},
		{0, 1, 2, 5},
		{0, 1, 5},
		{0, 5},
	}
	// Ensure all generatedTransitionSequence have a matching knownValidTransitionSequences
	for _, generatedTransitionSequence := range allValidTransitionSequences {
		foundMatch := false
		for _, knownValidTransitionSequence := range knownValidTransitionSequences {
			if reflect.DeepEqual(generatedTransitionSequence, knownValidTransitionSequence) {
				foundMatch = true
			}
		}
		assert.True(t, foundMatch)
	}
	// Ensure all knownValidTransitionSequences have a matching generatedTransitionSequence
	for _, knownValidTransitionSequence := range allValidTransitionSequences {
		foundMatch := false
		for _, generatedTransitionSequence := range allValidTransitionSequences {
			if reflect.DeepEqual(generatedTransitionSequence, knownValidTransitionSequence) {
				foundMatch = true
			}
		}
		assert.True(t, foundMatch)
	}
}

// TestQLDBSignedInteractions mocks QLDB to test signing and verifying of records that are
// persisted into QLDB
func TestQLDBSignedInteractions(t *testing.T) {
	testData := QLDBPaymentTransitionData{
		Status: Initialized,
	}
	marshaledData, err := json.Marshal(testData)
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
			Data:      marshaledData,
			Signature: []byte{},
		},
		Metadata: QLDBPaymentTransitionHistoryEntryMetadata{
			ID:      "test",
			Version: 1,
			TxTime:  time.Now(),
			TxID:    "test",
		},
	}
	binaryTransitionHistory, err := ion.MarshalBinary(mockTransitionHistory)
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	mockKMS := new(mockKMSClient)
	mockRes := new(mockResult)
	mockRes.On("GetCurrentData").Return(binaryTransitionHistory)
	mockDriver := new(MockDriver)
	mockDriver.On("Execute", context.Background(), mock.Anything).Return(mockRes, nil)
	mockKMS.On("Sign", context.Background(), mock.Anything, mock.Anything).Return(&kms.SignOutput{Signature: []byte("succeed")}, nil)
	mockKMS.On("Verify", context.Background(), mock.Anything, mock.Anything).Return(&kms.VerifyOutput{SignatureValid: true}, nil).Once()
	mockKMS.On("Verify", context.Background(), mock.Anything, mock.Anything).Return(&kms.VerifyOutput{SignatureValid: false}, nil)

	todoString := ""
	message := []byte("test")
	signingOutput, _ := mockKMS.Sign(ctx, &kms.SignInput{
		KeyId:            &todoString,
		Message:          message,
		SigningAlgorithm: kmsTypes.SigningAlgorithmSpecEcdsaSha256,
	})
	mockTransitionHistory.Data.Signature = signingOutput.Signature

	// First write should succeed because Verify returns true
	_, err = WriteQLDBObject(ctx, mockDriver, mockKMS, &mockTransitionHistory)
	assert.NoError(t, err)
	// Second write of the same object should fail because Verify returns false
	_, err = WriteQLDBObject(ctx, mockDriver, mockKMS, &mockTransitionHistory)
	assert.Error(t, err)
}

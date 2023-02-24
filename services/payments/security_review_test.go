package payments

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"reflect"
	"testing"
	"time"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/awslabs/amazon-qldb-driver-go/qldbdriver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Unit testing the package code using a Mock Driver
type MockDriver struct {
	mock.Mock
}

type mockResult struct {
	mock.Mock
}

type mockTransaction struct {
	mock.Mock
}

func (m *mockTransaction) Execute(statement string, parameters ...interface{}) (WrappedQldbResult, error) {
	args := m.Called(statement, parameters)
	return args.Get(0).(WrappedQldbResult), args.Error(1)
}

func (m *mockTransaction) BufferResult(res *qldbdriver.Result) (*qldbdriver.BufferedResult, error) {
	panic("not used")
}

func (m *mockTransaction) Abort() error {
	panic("not used")
}

func (m *MockDriver) Execute(ctx context.Context, fn func(txn qldbdriver.Transaction) (interface{}, error)) (interface{}, error) {
	args := m.Called(ctx)
	return args.Get(0).(*mockTransaction), args.Error(1)
}

func (m *MockDriver) Shutdown(ctx context.Context) {
	return
}

func (m *mockResult) GetCurrentData() []byte {
	args := m.Called()
	return args.Get(0).([]byte)
}
func (m *mockResult) Next(txn WrappedQldbTxnAPI) bool {
	args := m.Called(txn)
	return args.Get(0).(bool)
}

/*
Traverse QLDB history for a transaction and ensure that only valid transitions have occurred.
Should include exhaustive passing and failing tests.
*/
func TestVerifyPaymentTransitionHistory(t *testing.T) {
	// Valid transitions should be valid
	for _, transactionHistorySet := range transactionHistorySetTrue {
		valid, _ := TransitionHistoryIsValid(transactionHistorySet)
		assert.True(t, valid)
	}
	// Invalid transitions should be invalid
	for _, transactionHistorySet := range transactionHistorySetFalse {
		valid, _ := TransitionHistoryIsValid(transactionHistorySet)
		assert.False(t, valid)
	}
}

/*
Generate all valid transition sequences and ensure that this test contains the exact same set of
valid transition sequences. The purpose of this test is to alert us if outside changes
impact the set of valid transitions.
*/
func TestGenerateAllValidTransitions(t *testing.T) {
	allValidTransitionSequences := GetAllValidTransitionSequences()
	knownValidTransitionSequences := [][]QLDBPaymentTransitionState{
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
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	mockTransitionHistory := QLDBPaymentTransitionHistoryEntry{
		BlockAddress: QLDBPaymentTransitionHistoryEntryBlockAddress{
			StrandID:   "test",
			SequenceNo: 1,
		},
		Hash: "test",
		Data: QLDBPaymentTransitionHistoryEntryData{
			Status:    0,
			Signature: []byte{},
		},
		Metadata: QLDBPaymentTransitionHistoryEntryMetadata{
			ID:      "test",
			Version: 1,
			TxTime:  time.Now(),
			TxID:    "test",
		},
	}
	mockTransitionHistory.Data.Signature = ed25519.Sign(priv, mockTransitionHistory.BuildSigningBytes())
	binaryTransitionHistory, err := ion.MarshalBinary(mockTransitionHistory)
	if err != nil {
		panic(err)
	}
	mockTxn := new(mockTransaction)
	mockRes := new(mockResult)
	mockRes.On("Next", mockTxn).Return(true).Once()
	mockRes.On("Next", mockTxn).Return(false)
	mockRes.On("GetCurrentData").Return(binaryTransitionHistory)
	mockDriver := new(MockDriver)
	mockTxn.On(
		"Execute",
		"SELECT * FROM history(PaymentTransitions) AS h WHERE h.metadata.id = 'SOME_ID'",
		mock.Anything,
	).Return(mockRes, nil)
	mockTxn.On(
		"Execute",
		"INSERT INTO PaymentTransitions {'some_key': 'some_value'}",
		mock.Anything,
	).Return(mockRes, nil)
	mockDriver.On("Execute", context.Background(), mock.Anything).Return(mockTxn, nil)

	// Mock write data
	_, err = WriteQLDBObject(mockDriver, priv, mockTransitionHistory)
	if err != nil {
		panic(err)
	}
	signedBytes := mockTransitionHistory.BuildSigningBytes()

	// Mock read data
	fetched, _ := GetQLDBObject(mockTxn, "")
	assert.True(t, ed25519.Verify(pub, signedBytes, fetched.Data.Signature), nil)
}

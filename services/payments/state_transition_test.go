package payments

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"

	"github.com/aws/aws-sdk-go-v2/service/qldb"
	qldbTypes "github.com/aws/aws-sdk-go-v2/service/qldb/types"
	"github.com/aws/smithy-go/middleware"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
	appctx "github.com/brave-intl/bat-go/libs/context"
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
func (m *mockResult) Next(txn qldbdriver.Transaction) bool {
	args := m.Called(txn)
	return args.Get(0).(bool)
}

func (m *mockSDKClient) New() *wrappedQldbSDKClient {
	args := m.Called()
	return args.Get(0).(*wrappedQldbSDKClient)
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
		trueObject    = QLDBPaymentTransitionHistoryEntry{
			BlockAddress: QLDBPaymentTransitionHistoryEntryBlockAddress{
				StrandID:   "strand1",
				SequenceNo: 10,
			},
			Hash: []byte("28G0yQD/5I1XW12lxjgEASX2XbD+PiRJS3bqmGRX2YY="),
			Data: paymentLib.PaymentState{
				Signature:          []byte{},
				UnsafePaymentState: []byte{},
			},
			Metadata: QLDBPaymentTransitionHistoryEntryMetadata{
				ID:      "transitionid1",
				Version: 10,
				TxTime:  time.Now(),
				TxID:    "",
			},
		}
		falseObject = QLDBPaymentTransitionHistoryEntry{
			BlockAddress: QLDBPaymentTransitionHistoryEntryBlockAddress{
				StrandID:   "strand2",
				SequenceNo: 10,
			},
			Hash: []byte("dGVzdGVzdGVzdAo="),
			Data: paymentLib.PaymentState{
				Signature:          []byte{},
				UnsafePaymentState: []byte{},
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
	ctx = context.WithValue(ctx, appctx.PaymentsQLDBLedgerNameCTXKey, "TEST_LEDGER")
	valid, err := revisionValidInTree(ctx, mockSDKClient, &trueObject)
	must.Equal(t, nil, err)
	should.True(t, valid)

	valid, err = revisionValidInTree(ctx, mockSDKClient, &falseObject)
	must.Equal(t, nil, err)
	should.False(t, valid)
}

// TestSortHashes tests that the sortHashes function returns different hashes in the
// correct order.
func TestSortHashes(t *testing.T) {
	hash1 := sha256.Sum256([]byte{1})
	hash2 := sha256.Sum256([]byte{2})
	hash3 := sha256.Sum256([]byte{3})
	hash4 := sha256.Sum256([]byte{4})
	concatenated21 := append(hash2[:], hash1[:]...)
	hash12 := sha256.Sum256(concatenated21)
	concatenated34 := append(hash4[:], hash3[:]...)
	hash34 := sha256.Sum256(concatenated34)

	// Ensure result order is as expected for these hashes
	hash2ShouldBeFirst, _ := sortHashes(hash1[:], hash2[:])
	should.Equal(t, [][]byte{hash2[:], hash1[:]}, hash2ShouldBeFirst)
	should.NotEqual(t, [][]byte{hash1[:], hash2[:]}, hash2ShouldBeFirst)

	hash4ShouldBeFirst, _ := sortHashes(hash3[:], hash4[:])
	should.Equal(t, [][]byte{hash4[:], hash3[:]}, hash4ShouldBeFirst)
	should.NotEqual(t, [][]byte{hash3[:], hash4[:]}, hash4ShouldBeFirst)

	hash34ShouldBeFirst, _ := sortHashes(hash34[:], hash12[:])
	should.Equal(t, [][]byte{hash34[:], hash12[:]}, hash34ShouldBeFirst)
	should.NotEqual(t, [][]byte{hash12[:], hash34[:]}, hash34ShouldBeFirst)

	// Same tests with different argument order to ensure it doesn't change results
	argSwapHash2ShouldBeFirst, _ := sortHashes(hash2[:], hash1[:])
	should.Equal(t, [][]byte{hash2[:], hash1[:]}, argSwapHash2ShouldBeFirst)
	should.NotEqual(t, [][]byte{hash1[:], hash2[:]}, argSwapHash2ShouldBeFirst)

	argSwapHash4ShouldBeFirst, _ := sortHashes(hash4[:], hash3[:])
	should.Equal(t, [][]byte{hash4[:], hash3[:]}, argSwapHash4ShouldBeFirst)
	should.NotEqual(t, [][]byte{hash3[:], hash4[:]}, argSwapHash4ShouldBeFirst)

	argSwapHash34ShouldBeFirst, _ := sortHashes(hash12[:], hash34[:])
	should.Equal(t, [][]byte{hash34[:], hash12[:]}, argSwapHash34ShouldBeFirst)
	should.NotEqual(t, [][]byte{hash12[:], hash34[:]}, argSwapHash34ShouldBeFirst)
}

func TestVerifyHashSequence(t *testing.T) {
	hash1 := sha256.Sum256([]byte{1})
	hash2 := sha256.Sum256([]byte{2})
	hash3 := sha256.Sum256([]byte{3})
	hash4 := sha256.Sum256([]byte{4})
	concatenated21 := append(hash2[:], hash1[:]...)
	hash12 := sha256.Sum256(concatenated21)
	concatenated34 := append(hash4[:], hash3[:]...)
	hash34 := sha256.Sum256(concatenated34)
	concatenatedDigest := append(hash34[:], hash12[:]...)
	testDigest := sha256.Sum256(concatenatedDigest)
	base64Digest := []byte(base64.StdEncoding.EncodeToString(testDigest[:]))
	tipAddress := "1234"
	testProofIonText := [][32]byte{hash1, hash34}

	var (
		trueInitialHash  QLDBPaymentTransitionHistoryEntryHash = []byte("28G0yQD/5I1XW12lxjgEASX2XbD+PiRJS3bqmGRX2YY=")
		falseInitialHash QLDBPaymentTransitionHistoryEntryHash = []byte("dGVzdGVzdGVzdAo=")
	)

	testDigestOutput := qldb.GetDigestOutput{
		Digest:           base64Digest,
		DigestTipAddress: &qldbTypes.ValueHolder{IonText: &tipAddress},
		ResultMetadata:   middleware.Metadata{},
	}

	valid, err := verifyHashSequence(&testDigestOutput, trueInitialHash, testProofIonText)
	must.Equal(t, nil, err)
	should.True(t, valid)

	valid, err = verifyHashSequence(&testDigestOutput, falseInitialHash, testProofIonText)
	must.Equal(t, nil, err)
	should.False(t, valid)
}

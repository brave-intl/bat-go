package payments

import (
	"context"
	"crypto/sha256"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/mock"
	must "github.com/stretchr/testify/require"
)

type mockKMSClient struct {
	mock.Mock
}

func (m mockKMSClient) Sign(ctx context.Context, params *kms.SignInput, optFns ...func(*kms.Options)) (*kms.SignOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*kms.SignOutput), args.Error(1)
}

func (m mockKMSClient) Verify(ctx context.Context, params *kms.VerifyInput, optFns ...func(*kms.Options)) (*kms.VerifyOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*kms.VerifyOutput), args.Error(1)
}

func (m mockKMSClient) GetPublicKey(ctx context.Context, params *kms.GetPublicKeyInput, optFns ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*kms.GetPublicKeyOutput), args.Error(1)
}

/*
TestValidatePaymentStateSignatures
*/
func TestValidatePaymentStateSignatures(t *testing.T) {
	privkey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must.Equal(t, nil, err)
	pubkeyMarshalled := elliptic.MarshalCompressed(elliptic.P256(), privkey.X, privkey.Y)
	fmt.Printf("%v\n", pubkeyMarshalled)
	pubkeyHex := hex.EncodeToString(pubkeyMarshalled)

	idempotencyKey, err := uuid.Parse("1803df27-f29c-537a-9384-bb5b523ea3f7")
	must.Equal(t, nil, err)

	testTransaction := AuthenticatedPaymentState{
		Status: Prepared,
		PaymentDetails: PaymentDetails{
			Amount:    decimal.NewFromFloat(1.1),
			Custodian: "bitflyer",
		},
		Authorizations: []PaymentAuthorization{{}, {}, {}},
	}

	marshaledDataJSON, err := json.Marshal(testTransaction)
	must.Equal(t, nil, err)
	//	marshaledDataIon, err := ion.MarshalBinary(testTransaction)
	//	must.Equal(t, nil, err)

	hash := sha256.New()
	hash.Write(marshaledDataJSON)
	signature, err := ecdsa.SignASN1(rand.Reader, privkey, hash.Sum(nil))
	must.Equal(t, nil, err)

	mockTransitionHistory := QLDBPaymentTransitionHistoryEntry{
		BlockAddress: QLDBPaymentTransitionHistoryEntryBlockAddress{
			StrandID:   "test",
			SequenceNo: 1,
		},
		Data: PaymentState{
			UnsafePaymentState: marshaledDataJSON,
			Signature:          signature,
			ID:                 idempotencyKey,
			PublicKey:          []byte(pubkeyHex),
		},
		Metadata: QLDBPaymentTransitionHistoryEntryMetadata{
			ID:      "test",
			Version: 1,
			TxTime:  time.Now(),
			TxID:    "test",
		},
	}
	mockKMS := new(mockKMSClient)
	mockKMS.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(&kms.VerifyOutput{SignatureValid: false}, nil)

	verificationResult, err := validatePaymentStateSignatures(
		context.TODO(),
		mockKMS,
		"",
		[]QLDBPaymentTransitionHistoryEntry{mockTransitionHistory},
	)
	must.Equal(t, nil, err)
	must.True(t, verificationResult)
}

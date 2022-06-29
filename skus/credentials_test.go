package skus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	kafkautils "github.com/brave-intl/bat-go/utils/kafka"
	mockdialer "github.com/brave-intl/bat-go/utils/kafka/mock"
	testutils "github.com/brave-intl/bat-go/utils/test"
	"github.com/golang/mock/gomock"
	"github.com/linkedin/goavro"
	uuid "github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDeduplicateCredentialBindings(t *testing.T) {

	var tokens = []CredentialBinding{
		{
			TokenPreimage: "totally_random",
		},
		{
			TokenPreimage: "totally_random_1",
		},
		{
			TokenPreimage: "totally_random",
		},
		{
			TokenPreimage: "totally_random_2",
		},
	}
	var seen = []CredentialBinding{}

	var result = DeduplicateCredentialBindings(tokens...)
	if len(result) > len(tokens) {
		t.Error("result should be less than number of tokens")
	}

	for _, v := range result {
		for _, vv := range seen {
			if v == vv {
				t.Error("Deduplication of tokens didn't work")
			}
			seen = append(seen, v)
		}
	}
}

func TestIssuerID(t *testing.T) {

	cases := []struct {
		MerchantID string
		SKU        string
	}{
		{
			MerchantID: "brave.com",
			SKU:        "anon-card-vote",
		},
		{
			MerchantID: "",
			SKU:        "anon-card-vote",
		},
		{
			MerchantID: "brave.com",
			SKU:        "",
		},
		{
			MerchantID: "",
			SKU:        "",
		},
	}

	for _, v := range cases {

		issuerID, err := encodeIssuerID(v.MerchantID, v.SKU)
		fmt.Println(issuerID)
		if err != nil {
			t.Error("failed to encode: ", err)
		}

		merchantIDPrime, skuPrime, err := decodeIssuerID(issuerID)
		if err != nil {
			t.Error("failed to encode: ", err)
		}

		if v.MerchantID != merchantIDPrime {
			t.Errorf(
				"merchantID does not match decoded: %s != %s", v.MerchantID, merchantIDPrime)
		}

		if v.SKU != skuPrime {
			t.Errorf(
				"sku does not match decoded: %s != %s", v.SKU, skuPrime)
		}
	}
}

func TestFetchSignedOrderCredentials_KafkaError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	kafkaReader := mockdialer.NewMockKafkaReader(ctrl)

	ctx := context.Background()
	err := errors.New(uuid.NewV4().String())

	kafkaReader.EXPECT().
		ReadMessage(gomock.Eq(ctx)).
		Return(kafka.Message{}, err)

	s := Service{
		kafkaOrderCredsSignedRequestReader: kafkaReader,
	}

	expected := fmt.Errorf("read message: error reading kafka message %w", err)

	signingOrderResult, actual := s.FetchSignedOrderCredentials(ctx)

	assert.Nil(t, signingOrderResult)
	assert.EqualError(t, actual, expected.Error())
}

func TestFetchSignedOrderCredentials_CodecError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	kafkaReader := mockdialer.NewMockKafkaReader(ctrl)

	ctx := context.Background()

	kafkaReader.EXPECT().
		ReadMessage(gomock.Eq(ctx)).
		Return(kafka.Message{}, nil)

	codec := make(map[string]*goavro.Codec)

	s := Service{
		codecs:                             codec,
		kafkaOrderCredsSignedRequestReader: kafkaReader,
	}

	expected := fmt.Errorf("read message: could not find codec %s", kafkaSignedOrderCredsTopic)

	signingOrderResult, actual := s.FetchSignedOrderCredentials(ctx)

	assert.Nil(t, signingOrderResult)
	assert.EqualError(t, actual, expected.Error())
}

func TestFetchSignedOrderCredentials_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	codecs, err := kafkautils.GenerateCodecs(map[string]string{
		kafkaSignedOrderCredsTopic: signingOrderResultSchema,
	})
	require.NoError(t, err)

	ctx := context.Background()
	msg := makeMsg()

	textual, err := json.Marshal(msg)
	require.NoError(t, err)

	native, _, err := codecs[kafkaSignedOrderCredsTopic].NativeFromTextual(textual)
	require.NoError(t, err)

	binary, err := codecs[kafkaSignedOrderCredsTopic].BinaryFromNative(nil, native)
	require.NoError(t, err)

	message := kafka.Message{
		Key:   []byte(uuid.NewV4().String()),
		Value: binary,
	}

	kafkaReader := mockdialer.NewMockKafkaReader(ctrl)
	kafkaReader.EXPECT().
		ReadMessage(gomock.Eq(ctx)).
		Return(message, nil)

	s := Service{
		codecs:                             codecs,
		kafkaOrderCredsSignedRequestReader: kafkaReader,
	}

	actual, err := s.FetchSignedOrderCredentials(ctx)
	require.NoError(t, err)

	assert.Equal(t, msg, actual)
}

func makeMsg() *SigningOrderResult {
	return &SigningOrderResult{
		RequestID: testutils.RandomString(),
		Data: []SignedOrder{
			{
				PublicKey:      testutils.RandomString(),
				Proof:          testutils.RandomString(),
				Status:         SignedOrderStatusOk,
				SignedTokens:   []string{testutils.RandomString()},
				AssociatedData: []byte{},
			},
		},
	}
}

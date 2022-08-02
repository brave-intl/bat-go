package skus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/brave-intl/bat-go/utils/backoff"
	"github.com/brave-intl/bat-go/utils/clients"
	"net/http"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/utils/clients/cbr"
	mock_cbr "github.com/brave-intl/bat-go/utils/clients/cbr/mock"
	kafkautils "github.com/brave-intl/bat-go/utils/kafka"
	mockdialer "github.com/brave-intl/bat-go/utils/kafka/mock"
	"github.com/brave-intl/bat-go/utils/ptr"
	"github.com/brave-intl/bat-go/utils/test"
	"github.com/golang/mock/gomock"
	"github.com/linkedin/goavro"
	uuid "github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateIssuerV3_NewIssuer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	merchantID := "brave.com"

	orderItem := OrderItem{
		ID:          uuid.NewV4(),
		SKU:         test.RandomString(),
		ValidForISO: ptr.FromString("P1M"),
	}

	issuerID, err := encodeIssuerID(merchantID, orderItem.SKU)
	assert.NoError(t, err)

	issuerConfig := issuerConfig{
		buffer:  test.RandomInt(),
		overlap: test.RandomInt(),
	}

	// mock issuer calls
	cbrClient := mock_cbr.NewMockClient(ctrl)

	createIssuerV3 := cbr.IssuerRequest{
		Name:      issuerID,
		Cohort:    defaultCohort,
		MaxTokens: defaultMaxTokensPerIssuer,
		ValidFrom: ptr.FromTime(time.Now()),
		Duration:  *orderItem.ValidForISO,
		Buffer:    issuerConfig.buffer,
		Overlap:   issuerConfig.overlap,
	}
	cbrClient.EXPECT().
		CreateIssuerV3(ctx, isCreateIssuerV3(createIssuerV3)).
		Return(nil)

	issuerResponse := &cbr.IssuerResponse{
		Name:      issuerID,
		PublicKey: test.RandomString(),
	}
	cbrClient.EXPECT().
		GetIssuerV2(ctx, createIssuerV3.Name, createIssuerV3.Cohort).
		Return(issuerResponse, nil)

	// mock datastore
	datastore := NewMockDatastore(ctrl)

	datastore.EXPECT().
		GetIssuer(issuerID).
		Return(nil, nil)

	issuer := &Issuer{
		MerchantID: issuerResponse.Name,
		PublicKey:  issuerResponse.PublicKey,
	}
	datastore.EXPECT().
		InsertIssuer(issuer).
		Return(issuer, nil)

	// act, assert
	s := Service{
		cbClient:  cbrClient,
		Datastore: datastore,
		retry:     backoff.Retry,
	}

	err = s.CreateIssuerV3(ctx, merchantID, orderItem, issuerConfig)
	assert.NoError(t, err)
}

func TestCreateIssuerV3_AlreadyExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	merchantID := "brave.com"

	orderItem := OrderItem{
		ID:          uuid.NewV4(),
		SKU:         test.RandomString(),
		ValidForISO: ptr.FromString("P1M"),
	}

	issuerID, err := encodeIssuerID(merchantID, orderItem.SKU)
	assert.NoError(t, err)

	issuerConfig := issuerConfig{
		buffer:  test.RandomInt(),
		overlap: test.RandomInt(),
	}

	// mock datastore
	datastore := NewMockDatastore(ctrl)

	issuer := &Issuer{
		MerchantID: test.RandomString(),
		PublicKey:  test.RandomString(),
	}
	datastore.EXPECT().
		GetIssuer(issuerID).
		Return(issuer, nil)

	s := Service{
		Datastore: datastore,
	}

	err = s.CreateIssuerV3(ctx, merchantID, orderItem, issuerConfig)
	assert.NoError(t, err)
}

func TestCanRetry_True(t *testing.T) {
	httpError := clients.NewHTTPError(errors.New(test.RandomString()), test.RandomString(),
		test.RandomString(), http.StatusRequestTimeout, nil)
	f := canRetry(nonRetriableErrors)
	assert.True(t, f(httpError))
}

func TestCanRetry_False(t *testing.T) {
	httpError := clients.NewHTTPError(errors.New(test.RandomString()), test.RandomString(),
		test.RandomString(), http.StatusForbidden, nil)
	f := canRetry(nonRetriableErrors)
	assert.False(t, f(httpError))
}

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

	var result = deduplicateCredentialBindings(tokens...)
	if len(result) > len(tokens) {
		t.Error("result should be less than number of tokens")
	}

	var seen []CredentialBinding
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
		RequestID: test.RandomString(),
		Data: []SignedOrder{
			{
				PublicKey:      test.RandomString(),
				Proof:          test.RandomString(),
				Status:         SignedOrderStatusOk,
				SignedTokens:   []string{test.RandomString()},
				AssociatedData: []byte{},
				ValidFrom:      &UnionNullString{"string": time.Now().String()},
				ValidTo:        nil,
				BlindedTokens:  []string{test.RandomString()},
			},
		},
	}
}

func isCreateIssuerV3(expected cbr.IssuerRequest) gomock.Matcher {
	return createIssuerV3Matcher{expected: expected}
}

type createIssuerV3Matcher struct {
	expected cbr.IssuerRequest
}

func (c createIssuerV3Matcher) Matches(arg interface{}) bool {
	actual := arg.(cbr.IssuerRequest)
	return c.expected.Name == actual.Name &&
		c.expected.Cohort == actual.Cohort &&
		c.expected.MaxTokens == actual.MaxTokens &&
		c.expected.ValidFrom.Before(*actual.ValidFrom) &&
		c.expected.Duration == actual.Duration &&
		c.expected.Buffer == actual.Buffer &&
		c.expected.Overlap == actual.Overlap
}

func (c createIssuerV3Matcher) String() string {
	return "does not match"
}

//go:build integration

package skus

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/services/skus/skustest"
	"github.com/stretchr/testify/suite"

	"github.com/linkedin/goavro"

	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/clients/cbr"
	mock_cbr "github.com/brave-intl/bat-go/libs/clients/cbr/mock"
	"github.com/brave-intl/bat-go/libs/ptr"
	"github.com/brave-intl/bat-go/libs/test"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type CredentialsTestSuite struct {
	suite.Suite
	storage Datastore
}

func TestCredentialsTestSuite(t *testing.T) {
	suite.Run(t, new(CredentialsTestSuite))
}

func (suite *CredentialsTestSuite) SetupSuite() {
	skustest.Migrate(suite.T())
	storage, _ := NewPostgres("", false, "")
	suite.storage = storage
}

func (suite *CredentialsTestSuite) AfterTest() {
	skustest.CleanDB(suite.T(), suite.storage.RawDB())
}

func (suite *CredentialsTestSuite) TestSignedOrderCredentialsHandler_KafkaDuplicates() {
	// Create an issuer and a paid order with one order item and a time limited v2 credential type.
	ctx := context.WithValue(context.Background(), appctx.WhitelistSKUsCTXKey, []string{devFreeTimeLimitedV2})
	order, issuer := createOrderAndIssuer(suite.T(), ctx, suite.storage, devFreeTimeLimitedV2)

	requestID := uuid.NewV4()
	validTo := time.Now().Add(time.Hour)
	validFrom := time.Now().Local()

	// Insert a single signing order request.
	err := suite.storage.InsertSigningOrderRequestOutbox(context.Background(), requestID, order.ID,
		order.Items[0].ID, SigningOrderRequest{})
	suite.Require().NoError(err)

	// Create duplicate signed order results.
	var kafkaDuplicates []SigningOrderResult
	for i := 0; i < 10; i++ {
		kafkaDuplicates = append(kafkaDuplicates, suite.makeMsg(requestID, order.ID, order.Items[0].ID,
			issuer.ID, validTo, validFrom))
	}

	// Create Kafka messages from the duplicates signed order results.
	codec, err := goavro.NewCodec(signingOrderResultSchema)
	suite.Require().NoError(err)

	var kafkaMessages []kafka.Message
	for i := 0; i < len(kafkaDuplicates); i++ {
		b, err := json.Marshal(kafkaDuplicates[i])
		suite.Require().NoError(err)

		native, _, err := codec.NativeFromTextual(b)
		suite.Require().NoError(err)

		binary, err := codec.BinaryFromNative(nil, native)
		suite.Require().NoError(err)

		kafkaMessages = append(kafkaMessages, kafka.Message{
			Topic: kafkaUnsignedOrderCredsTopic,
			Value: binary,
		})
	}

	handler := SignedOrderCredentialsHandler{
		decoder:   &SigningOrderResultDecoder{codec: codec},
		datastore: suite.storage,
	}

	// Send them to handler with varied times and routines to mock different consumers.
	var wg sync.WaitGroup
	for i := 0; i < len(kafkaMessages); i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			time.Sleep(time.Millisecond * time.Duration(test.RandomIntWithMax(10)))
			_ = handler.Handle(context.Background(), kafkaMessages[index])
		}(i)
	}
	wg.Wait()

	creds, err := suite.storage.GetTimeLimitedV2OrderCredsByOrder(order.ID)
	suite.NoError(err)

	suite.Require().NotNil(creds)
	suite.Assert().Len(creds.Credentials, 1)
	suite.Assert().Equal(order.Items[0].ID, creds.Credentials[0].ItemID)
}

func (suite *CredentialsTestSuite) TestSignedOrderCredentialsHandler_RequestDuplicates() {
	// Create an issuer and a paid order with one order item and a time limited v2 credential type.
	ctx := context.WithValue(context.Background(), appctx.WhitelistSKUsCTXKey, []string{devFreeTimeLimitedV2})
	order, issuer := createOrderAndIssuer(suite.T(), ctx, suite.storage, devFreeTimeLimitedV2)

	// Insert a ten signing order requests for the same order and item id
	var requestIDs []uuid.UUID
	for i := 0; i < 10; i++ {
		requestIDs = append(requestIDs, uuid.NewV4())
		err := suite.storage.InsertSigningOrderRequestOutbox(context.Background(), requestIDs[i], order.ID,
			order.Items[0].ID, SigningOrderRequest{})
		suite.Require().NoError(err)
	}

	validTo := time.Now().Add(time.Hour)
	validFrom := time.Now().Local()

	// Create signed order results for all the request with the same order, order item and valid to and valid from.
	var signedOrderResults []SigningOrderResult
	for i := 0; i < len(requestIDs); i++ {
		signedOrderResults = append(signedOrderResults, suite.makeMsg(requestIDs[i], order.ID, order.Items[0].ID,
			issuer.ID, validTo, validFrom))
	}

	// Create Kafka messages from the signed order results.
	codec, err := goavro.NewCodec(signingOrderResultSchema)
	suite.Require().NoError(err)

	var kafkaMessages []kafka.Message
	for i := 0; i < len(signedOrderResults); i++ {
		b, err := json.Marshal(signedOrderResults[i])
		suite.Require().NoError(err)

		native, _, err := codec.NativeFromTextual(b)
		suite.Require().NoError(err)

		binary, err := codec.BinaryFromNative(nil, native)
		suite.Require().NoError(err)

		kafkaMessages = append(kafkaMessages, kafka.Message{
			Topic: kafkaUnsignedOrderCredsTopic,
			Value: binary,
		})
	}

	handler := SignedOrderCredentialsHandler{
		decoder:   &SigningOrderResultDecoder{codec: codec},
		datastore: suite.storage,
	}

	// Send them to handler with varied times and routines to mock different consumers.
	var wg sync.WaitGroup
	for i := 0; i < len(kafkaMessages); i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			time.Sleep(time.Millisecond * time.Duration(test.RandomIntWithMax(100)))
			_ = handler.Handle(context.Background(), kafkaMessages[index])
		}(i)
	}
	wg.Wait()

	creds, err := suite.storage.GetTimeLimitedV2OrderCredsByOrder(order.ID)
	suite.NoError(err)

	suite.Require().NotNil(creds)
	suite.Assert().Len(creds.Credentials, 1)
	suite.Assert().Equal(order.Items[0].ID, creds.Credentials[0].ItemID)
	// cleanup
	err = suite.storage.DeleteTimeLimitedV2OrderCredsByOrder(order.ID)
	suite.NoError(err)
}

func TestCreateIssuer_NewIssuer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	merchantID := "brave.com"

	orderItem := OrderItem{
		ID:                        uuid.NewV4(),
		SKU:                       test.RandomString(),
		ValidForISO:               ptr.FromString("P1M"),
		EachCredentialValidForISO: ptr.FromString("P1D"),
	}

	issuerID, err := encodeIssuerID(merchantID, orderItem.SKU)
	assert.NoError(t, err)

	// mock issuer calls
	cbrClient := mock_cbr.NewMockClient(ctrl)
	cbrClient.EXPECT().
		CreateIssuer(ctx, issuerID, defaultMaxTokensPerIssuer).
		Return(nil)

	issuerResponse := &cbr.IssuerResponse{
		Name:      issuerID,
		PublicKey: test.RandomString(),
	}
	cbrClient.EXPECT().
		GetIssuer(ctx, issuerID).
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

	err = s.CreateIssuer(ctx, merchantID, orderItem)
	assert.NoError(t, err)
}

func TestCreateIssuerV3_NewIssuer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	merchantID := "brave.com"

	orderItem := OrderItem{
		ID:                        uuid.NewV4(),
		SKU:                       test.RandomString(),
		ValidForISO:               ptr.FromString("P1M"),
		EachCredentialValidForISO: ptr.FromString("P1D"),
	}

	issuerID, err := encodeIssuerID(merchantID, orderItem.SKU)
	assert.NoError(t, err)

	issuerConfig := IssuerConfig{
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
		Duration:  *orderItem.EachCredentialValidForISO,
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
		GetIssuerV3(ctx, createIssuerV3.Name).
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

func TestCreateIssuer_AlreadyExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	merchantID := "brave.com"

	orderItem := OrderItem{
		ID:                        uuid.NewV4(),
		SKU:                       test.RandomString(),
		ValidForISO:               ptr.FromString("P1M"),
		EachCredentialValidForISO: ptr.FromString("P1D"),
	}

	issuerID, err := encodeIssuerID(merchantID, orderItem.SKU)
	assert.NoError(t, err)

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

	err = s.CreateIssuer(ctx, merchantID, orderItem)
	assert.NoError(t, err)
}

func TestCreateIssuerV3_AlreadyExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	merchantID := "brave.com"

	orderItem := OrderItem{
		ID:                        uuid.NewV4(),
		SKU:                       test.RandomString(),
		ValidForISO:               ptr.FromString("P1M"),
		EachCredentialValidForISO: ptr.FromString("P1D"),
	}

	issuerID, err := encodeIssuerID(merchantID, orderItem.SKU)
	assert.NoError(t, err)

	issuerConfig := IssuerConfig{
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

func TestCreateOrderCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	merchantID := "brave.com"

	orderItem := OrderItem{
		ID:                        uuid.NewV4(),
		SKU:                       test.RandomString(),
		ValidForISO:               ptr.FromString("P1M"),
		EachCredentialValidForISO: ptr.FromString("P1D"),
	}

	issuerID, err := encodeIssuerID(merchantID, orderItem.SKU)
	assert.NoError(t, err)

	issuerConfig := IssuerConfig{
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

func TestDecodeSignedOrderCredentials_Success(t *testing.T) {
	codec, err := goavro.NewCodec(signingOrderResultSchema)
	require.NoError(t, err)

	msg := &SigningOrderResult{
		RequestID: test.RandomString(),
		Data: []SignedOrder{
			{
				PublicKey:      test.RandomString(),
				Proof:          test.RandomString(),
				Status:         SignedOrderStatusOk,
				SignedTokens:   []string{test.RandomString()},
				AssociatedData: []byte{},
				ValidFrom:      &UnionNullString{"string": time.Now().Local().Format(time.RFC3339)},
				ValidTo:        &UnionNullString{"string": time.Now().Add(1 * time.Hour).Local().Format(time.RFC3339)},
				BlindedTokens:  []string{test.RandomString()},
			},
		},
	}

	textual, err := json.Marshal(msg)
	require.NoError(t, err)

	native, _, err := codec.NativeFromTextual(textual)
	require.NoError(t, err)

	binary, err := codec.BinaryFromNative(nil, native)
	require.NoError(t, err)

	message := kafka.Message{
		Key:   []byte(uuid.NewV4().String()),
		Value: binary,
	}

	d := SigningOrderResultDecoder{
		codec: codec,
	}

	actual, err := d.Decode(message)
	require.NoError(t, err)

	assert.Equal(t, msg, actual)
}

func (suite *CredentialsTestSuite) makeMsg(requestID, orderID, itemID, issuerID uuid.UUID,
	to, from time.Time) SigningOrderResult {

	metadata := Metadata{
		ItemID:         itemID,
		OrderID:        orderID,
		IssuerID:       issuerID,
		CredentialType: timeLimitedV2,
	}

	associatedData, err := json.Marshal(metadata)
	suite.Require().NoError(err)

	return SigningOrderResult{
		RequestID: requestID.String(),
		Data: []SignedOrder{
			{
				PublicKey:      test.RandomString(),
				Proof:          test.RandomString(),
				Status:         SignedOrderStatusOk,
				BlindedTokens:  []string{test.RandomString()},
				SignedTokens:   []string{test.RandomString()},
				ValidTo:        &UnionNullString{"string": to.Format(time.RFC3339)},
				ValidFrom:      &UnionNullString{"string": from.Format(time.RFC3339)},
				AssociatedData: associatedData,
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

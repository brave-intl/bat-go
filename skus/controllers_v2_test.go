//go:build integration

package skus

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/brave-intl/bat-go/utils/backoff"
	"github.com/brave-intl/bat-go/utils/backoff/retrypolicy"
	mock_cbr "github.com/brave-intl/bat-go/utils/clients/cbr/mock"
	"github.com/brave-intl/bat-go/utils/datastore"
	"github.com/golang/mock/gomock"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/cmd/macaroon"
	"github.com/brave-intl/bat-go/skus/skustest"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
	kafkautils "github.com/brave-intl/bat-go/utils/kafka"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/brave-intl/bat-go/utils/test"
	"github.com/linkedin/goavro"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
)

type ControllersV2TestSuite struct {
	suite.Suite
	storage Datastore
}

func TestControllersV2TestSuite(t *testing.T) {
	suite.Run(t, new(ControllersV2TestSuite))
}

func (suite *ControllersV2TestSuite) SetupSuite() {
	skustest.Migrate(suite.T())
	storage, _ := NewPostgres("", false, "")
	suite.storage = storage
}

func (suite *ControllersV2TestSuite) AfterTest() {
	skustest.CleanDB(suite.T(), suite.storage.RawDB())
}

func (suite *ControllersV2TestSuite) TestCreateOrderCredsV2_NewSku() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	env := os.Getenv("ENV")
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, env)

	// setup kafka
	kafkaUnsignedOrderCredsTopic = test.RandomString()
	kafkaSignedOrderCredsTopic = test.RandomString()
	kafkaOrderCredsSignedRequestReaderGroupID = test.RandomString()
	ctx = skustest.SetupKafka(suite.T(), ctx, kafkaUnsignedOrderCredsTopic)

	// create macaroon token for sku and whitelist
	sku := test.RandomString()
	price := 0
	token := suite.CreateMacaroon(sku, price)
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{token})

	// create order with order items
	request := CreateOrderRequest{
		Email: test.RandomString(),
		Items: []OrderItemRequest{
			{
				SKU:      token,
				Quantity: 1,
			},
			{
				SKU:      token,
				Quantity: 1,
			},
		},
	}

	// stub create issuers calls
	merchantID := "brave.com"
	issuerID, err := encodeIssuerID(merchantID, sku)
	suite.Require().NoError(err)

	// mock issuer calls
	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	cbrClient := mock_cbr.NewMockClient(ctrl)
	cbrClient.EXPECT().
		CreateIssuerV3(ctx, gomock.AssignableToTypeOf(cbr.CreateIssuerV3Request{})).
		Return(nil)

	issuerResponse := &cbr.IssuerResponse{
		Name:      issuerID,
		PublicKey: test.RandomString(),
	}
	cbrClient.EXPECT().
		GetIssuer(ctx, issuerID).
		Return(issuerResponse, nil)

	retryPolicy = retrypolicy.NoRetry // set this so we fail fast

	service := Service{Datastore: suite.storage, cbClient: cbrClient, retry: backoff.Retry}
	order, err := service.CreateOrderFromRequest(ctx, request)
	suite.Require().NoError(err)

	// create order creds v2 request
	data := CreateOrderCredsV2Request{
		ItemID:       order.Items[0].ID,
		BlindedCreds: []string{base64.StdEncoding.EncodeToString([]byte(test.RandomString()))},
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	requestID := uuid.NewV4().String()
	ctx = context.WithValue(ctx, requestutils.RequestID, requestID)

	r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/%s/credentials",
		order.ID), bytes.NewBuffer(payload)).WithContext(ctx)

	rw := httptest.NewRecorder()

	instrumentHandler := func(name string, h http.Handler) http.Handler {
		return h
	}

	skuService, err := InitService(ctx, suite.storage, nil)
	suite.Require().NoError(err)

	router := RouterV2(skuService, instrumentHandler)

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, r)

	// assert we have written unsigned order creds to kafka
	signingOrderRequest := suite.ReadSigningOrderRequestMessage(ctx, kafkaUnsignedOrderCredsTopic)

	suite.Require().Equal(requestID, signingOrderRequest.RequestID)
	suite.Require().Equal(order.Items[0].SKU, signingOrderRequest.Data[0].IssuerType)
	suite.Require().Equal(defaultCohort, signingOrderRequest.Data[0].IssuerCohort)

	var metadata datastore.Metadata
	err = json.Unmarshal(signingOrderRequest.Data[0].AssociatedData, &metadata)
	suite.Require().NoError(err)

	suite.Require().Equal(order.ID.String(), metadata["order_id"])
	suite.Require().Equal(order.Items[0].ID.String(), metadata["item_id"])

	suite.Assert().Equal(http.StatusCreated, rw.Code)
}

func (suite *ControllersV2TestSuite) TestCreateOrderCredsV2_Order_Unpaid() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	env := os.Getenv("ENV")
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, env)

	// create unpaid order
	// create macaroon token for sku and whitelist
	sku := test.RandomString()
	price := test.RandomNonZeroInt()
	token := suite.CreateMacaroon(sku, price)
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{token})

	// create order with order items
	request := CreateOrderRequest{
		Email: test.RandomString(),
		Items: []OrderItemRequest{
			{
				SKU:      token,
				Quantity: 1,
			},
		},
	}

	// stub create issuers calls
	merchantID := "brave.com"
	issuerID, err := encodeIssuerID(merchantID, sku)
	suite.Require().NoError(err)

	// mock issuer calls
	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	cbrClient := mock_cbr.NewMockClient(ctrl)
	cbrClient.EXPECT().
		CreateIssuerV3(ctx, gomock.AssignableToTypeOf(cbr.CreateIssuerV3Request{})).
		Return(nil)

	issuerResponse := &cbr.IssuerResponse{
		Name:      issuerID,
		PublicKey: test.RandomString(),
	}
	cbrClient.EXPECT().
		GetIssuer(ctx, issuerID).
		Return(issuerResponse, nil)

	// create orders
	retryPolicy = retrypolicy.NoRetry // set this so we fail fast

	service := Service{Datastore: suite.storage, cbClient: cbrClient, retry: backoff.Retry}
	order, err := service.CreateOrderFromRequest(ctx, request)
	suite.Require().NoError(err)

	// create order creds v2 request
	data := CreateOrderCredsV2Request{
		ItemID:       order.Items[0].ID,
		BlindedCreds: []string{base64.StdEncoding.EncodeToString([]byte(test.RandomString()))},
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	requestID := uuid.NewV4().String()
	ctx = context.WithValue(ctx, requestutils.RequestID, requestID)

	r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/%s/credentials",
		order.ID), bytes.NewBuffer(payload)).WithContext(ctx)

	rw := httptest.NewRecorder()

	instrumentHandler := func(name string, h http.Handler) http.Handler {
		return h
	}

	skuService, err := InitService(ctx, suite.storage, nil)
	suite.Require().NoError(err)

	router := RouterV2(skuService, instrumentHandler)

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, r)

	expected := handlers.AppError{
		Cause:   nil,
		Message: "error creating order credentials: order not paid",
		Code:    http.StatusBadRequest,
	}

	var appError handlers.AppError
	err = json.NewDecoder(rw.Body).Decode(&appError)

	suite.Require().Equal(http.StatusBadRequest, rw.Code)
	suite.Require().Equal(expected, appError)
}

func (suite *ControllersV2TestSuite) TestCreateOrderCredsV2_Order_NotFound() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	data := CreateOrderCredsV2Request{
		ItemID:       uuid.NewV4(),
		BlindedCreds: []string{base64.StdEncoding.EncodeToString([]byte(test.RandomString()))},
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	requestID := uuid.NewV4().String()
	ctx = context.WithValue(ctx, requestutils.RequestID, requestID)

	r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/%s/credentials",
		uuid.NewV4()), bytes.NewBuffer(payload)).WithContext(ctx)

	rw := httptest.NewRecorder()

	instrumentHandler := func(name string, h http.Handler) http.Handler {
		return h
	}

	skuService, err := InitService(ctx, suite.storage, nil)
	suite.Require().NoError(err)

	router := RouterV2(skuService, instrumentHandler)

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, r)

	var appError handlers.AppError
	err = json.NewDecoder(rw.Body).Decode(&appError)

	suite.Require().Equal(http.StatusNotFound, rw.Code)
	suite.Require().Contains(appError.Message, errorutils.ErrNotFound.Error())
}

func (suite *ControllersV2TestSuite) ReadSigningOrderRequestMessage(ctx context.Context, topic string) SigningOrderRequest {
	kafkaReader, err := kafkautils.NewKafkaReader(ctx, test.RandomString(), topic)
	suite.Require().NoError(err)

	msg, err := kafkaReader.ReadMessage(ctx)
	suite.Require().NoError(err)

	codec, err := goavro.NewCodec(signingOrderRequestSchema)
	suite.Require().NoError(err)

	native, _, err := codec.NativeFromBinary(msg.Value)
	suite.Require().NoError(err)

	textual, err := codec.TextualFromNative(nil, native)
	suite.Require().NoError(err)

	var signingOrderRequest SigningOrderRequest
	err = json.Unmarshal(textual, &signingOrderRequest)
	suite.Require().NoError(err)

	return signingOrderRequest
}

// To create an unpaid order item set price to 0
func (suite *ControllersV2TestSuite) CreateMacaroon(sku string, price int) string {
	c := macaroon.Caveats{
		"sku":                       sku,
		"price":                     strconv.Itoa(price),
		"description":               test.RandomString(),
		"currency":                  "usd",
		"credential_type":           "time-limited-v2",
		"credential_valid_duration": "P1M",
		"issuer_token_buffer":       strconv.Itoa(test.RandomInt()),
		"issuer_token_overlap":      strconv.Itoa(test.RandomInt()),
		"allowed_payment_methods":   test.RandomString(),
		"metadata": `
				{
					"stripe_product_id":"stripe_product_id",
					"stripe_success_url":"stripe_success_url",
					"stripe_cancel_url":"stripe_cancel_url"
				}
			`,
	}

	t := macaroon.Token{
		ID: test.RandomString(), Version: 1, Location: "brave.com",
		FirstPartyCaveats: []macaroon.Caveats{c},
	}

	mac, err := t.Generate("secret")
	suite.Require().NoError(err)

	skuMap["development"][mac] = true

	return mac
}

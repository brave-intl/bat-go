//go:build integration

package skus

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/utils/datastore"

	"github.com/brave-intl/bat-go/utils/clients/cbr"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/jsonutils"
	kafkautils "github.com/brave-intl/bat-go/utils/kafka"
	"github.com/brave-intl/bat-go/utils/ptr"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/brave-intl/bat-go/utils/test"
	"github.com/linkedin/goavro"
	uuid "github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
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
	storage, err := NewPostgres("", true, "skus_db")
	suite.Require().NoError(err)

	m, err := storage.NewMigrate()
	suite.Require().NoError(err)

	ver, dirty, _ := m.Version()
	if dirty {
		suite.Require().NoError(m.Force(int(ver)))
	}

	if ver > 0 {
		suite.Require().NoError(m.Down())
	}

	err = storage.Migrate()
	suite.Require().NoError(err)

	suite.storage = storage
}

func (suite *ControllersV2TestSuite) AfterTest() {
	suite.CleanDB()
}

func (suite *ControllersV2TestSuite) CleanDB() {
	tables := []string{"vote_drain", "api_keys", "transactions", "order_creds", "order_cred_issuers",
		"order_items", "orders"}

	for _, table := range tables {
		_, err := suite.storage.RawDB().Exec("delete from " + table)
		suite.Require().NoError(err)
	}
}

func (suite *ControllersV2TestSuite) TestCreateOrderCredsV2_Created_New_SKU() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := os.Getenv("ENV")
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, env)

	// stub create issuers v3 call
	ts := suite.stubCreateIssuerV3Endpoint()
	err := os.Setenv("CHALLENGE_BYPASS_SERVER", ts.URL)
	suite.Require().NoError(err)

	// setup kafka
	kafkaUnsignedOrderCredsTopic = test.RandomString()
	kafkaSignedOrderCredsTopic = test.RandomString()
	KafkaOrderCredsSignedRequestReaderGroupID = test.RandomString()
	ctx = suite.setupKafka(ctx, kafkaUnsignedOrderCredsTopic)

	// create paid order
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{devBraveFirewallVPNPremiumTimeLimited, devBraveSearchPremiumYearTimeLimited})

	service := Service{}
	orderItem, methods, err := service.CreateOrderItemFromMacaroon(ctx, devBraveFirewallVPNPremiumTimeLimited, 1)
	suite.Require().NoError(err)

	order, err := suite.storage.CreateOrder(decimal.NewFromInt32(int32(test.RandomInt())), test.RandomString(), OrderStatusPaid,
		test.RandomString(), test.RandomString(), nil, []OrderItem{*orderItem}, methods)
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
	suite.Require().Equal(orderItem.SKU, signingOrderRequest.Data[0].IssuerType)
	suite.Require().Equal(cohort, signingOrderRequest.Data[0].IssuerCohort)

	var metadata datastore.Metadata
	err = json.Unmarshal(signingOrderRequest.Data[0].AssociatedData, &metadata)
	suite.Require().NoError(err)

	suite.Require().Equal(order.ID.String(), metadata["order_id"])
	suite.Require().Equal(order.Items[0].ID.String(), metadata["item_id"])

	suite.Assert().Equal(http.StatusCreated, rw.Code)
}

func (suite *ControllersV2TestSuite) TestRunStoreSignedOrderCredentialsJob() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := os.Getenv("ENV")
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, env)

	// create paid order and insert order creds
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{devBraveSearchPremiumYearTimeLimited})
	order := suite.createOrder(ctx, devBraveSearchPremiumYearTimeLimited)

	// setup kafka and write expected signed creds to topic. Overwrite topics so fresh for each test
	kafkaSignedOrderCredsTopic = test.RandomString()
	KafkaOrderCredsSignedRequestReaderGroupID = test.RandomString()
	ctx = suite.setupKafka(ctx, kafkaSignedOrderCredsTopic)

	associatedData := make(map[string]string)
	associatedData["order_id"] = order.ID.String()
	associatedData["item_id"] = order.Items[0].ID.String()
	associatedData["valid_to"] = time.Now().String()
	associatedData["valid_from"] = time.Now().String()

	b, err := json.Marshal(associatedData)
	suite.Require().NoError(err)

	signingOrderResult := SigningOrderResult{
		RequestID: uuid.NewV4().String(),
		Data: []SignedOrder{
			{
				PublicKey:      test.RandomString(),
				Proof:          test.RandomString(),
				Status:         SignedOrderStatusOk,
				SignedTokens:   []string{test.RandomString()},
				AssociatedData: b,
			},
		},
	}
	suite.WriteSigningOrderResultMessage(ctx, signingOrderResult, kafkaSignedOrderCredsTopic)

	// act
	go func() {
		service, _ := InitService(ctx, suite.storage, nil)
		_, err = service.RunStoreSignedOrderCredentialsJob(ctx)
	}()

	time.Sleep(5 * time.Second)

	// assert
	actual, err := suite.storage.GetOrderTimeLimitedV2CredsByItemID(order.ID, order.Items[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(actual)

	suite.Assert().Equal(order.ID, actual.Credentials[0].OrderID)
	suite.Assert().Equal(jsonutils.JSONStringArray(signingOrderResult.Data[0].SignedTokens), *actual.Credentials[0].SignedCreds)
	suite.Assert().Equal(signingOrderResult.Data[0].PublicKey, *actual.Credentials[0].PublicKey)
	suite.Assert().Equal(signingOrderResult.Data[0].Proof, *actual.Credentials[0].BatchProof)
}

func (suite *ControllersV2TestSuite) stubCreateIssuerV3Endpoint() *httptest.Server {
	suite.T().Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/v1/issuer"):
			// get issuer
			suite.Equal(http.MethodGet, r.Method)

			resp := cbr.IssuerResponse{
				Name:      test.RandomString(),
				PublicKey: test.RandomString(),
			}

			b, err := json.Marshal(resp)
			suite.Require().NoError(err)

			w.WriteHeader(http.StatusOK)

			_, err = w.Write(b)
			suite.Require().NoError(err)

		case strings.Contains(r.URL.Path, "/v3/issuer/"):
			// create issuer
			suite.Equal(http.MethodPost, r.Method)
			w.WriteHeader(http.StatusCreated)
		default:
			suite.Fail("unknown url path")
		}
	}))

	return ts
}

func (suite *ControllersV2TestSuite) setupKafka(ctx context.Context, topic string) context.Context {
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	ctx = context.WithValue(ctx, appctx.KafkaBrokersCTXKey, kafkaBrokers)

	dialer, _, err := kafkautils.TLSDialer()
	suite.Require().NoError(err)
	conn, err := dialer.DialLeader(ctx, "tcp", strings.Split(kafkaBrokers, ",")[0], topic, 0)
	suite.Require().NoError(err)

	err = conn.CreateTopics(kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1})
	suite.Require().NoError(err)

	return ctx
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

func (suite *ControllersV2TestSuite) WriteSigningOrderResultMessage(ctx context.Context, signingOrderResult SigningOrderResult, topic string) {
	codec, err := goavro.NewCodec(signingOrderResultSchema)
	suite.Require().NoError(err)

	textual, err := json.Marshal(signingOrderResult)
	suite.Require().NoError(err)

	native, _, err := codec.NativeFromTextual(textual)
	suite.Require().NoError(err)

	binary, err := codec.BinaryFromNative(nil, native)
	suite.Require().NoError(err)

	kafkaWriter, _, err := kafkautils.InitKafkaWriter(ctx, "")
	suite.Require().NoError(err)

	err = kafkaWriter.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(signingOrderResult.RequestID),
		Value: binary,
	})
	suite.Require().NoError(err)
}

func (suite *ControllersV2TestSuite) createOrder(ctx context.Context, sku string) *Order {
	service := Service{}
	orderItem, method, err := service.CreateOrderItemFromMacaroon(ctx, sku, 1)
	suite.Require().NoError(err)

	order, err := suite.storage.CreateOrder(decimal.NewFromInt32(int32(test.RandomInt())), test.RandomString(), OrderStatusPaid,
		test.RandomString(), test.RandomString(), nil, []OrderItem{*orderItem}, method)
	suite.Require().NoError(err)

	// create issuer
	pk := test.RandomString()

	issuer := &Issuer{
		MerchantID: test.RandomString(),
		PublicKey:  pk,
	}

	issuer, err = suite.storage.InsertIssuer(issuer)
	suite.Require().NoError(err)

	// insert order creds
	oc := &OrderCreds{
		ID:           order.Items[0].ID, // item_id
		OrderID:      order.ID,
		IssuerID:     issuer.ID,
		BlindedCreds: nil,
		BatchProof:   ptr.FromString(test.RandomString()),
		PublicKey:    ptr.FromString(pk),
	}
	err = suite.storage.InsertOrderCreds(oc)
	suite.Require().NoError(err)

	return order
}

//go:build integration

package skus

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/go-chi/chi"
	"github.com/go-chi/cors"
	"github.com/golang/mock/gomock"
	"github.com/linkedin/goavro"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/libs/backoff/retrypolicy"
	"github.com/brave-intl/bat-go/libs/clients/cbr"
	mockcb "github.com/brave-intl/bat-go/libs/clients/cbr/mock"
	"github.com/brave-intl/bat-go/libs/clients/gemini"
	mockgemini "github.com/brave-intl/bat-go/libs/clients/gemini/mock"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	kafkautils "github.com/brave-intl/bat-go/libs/kafka"
	logutils "github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/requestutils"
	"github.com/brave-intl/bat-go/libs/test"
	timeutils "github.com/brave-intl/bat-go/libs/time"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/services/skus/handler"
	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/skustest"
	"github.com/brave-intl/bat-go/services/wallet"
	macaroon "github.com/brave-intl/bat-go/tools/macaroon/cmd"

	"github.com/brave-intl/bat-go/services/skus/storage/repository"
)

var (
	// these skus will be generated with the appropriate merchant key in setup
	UserWalletVoteToken             macaroon.Token
	UserWalletVoteTestSkuToken      string
	UserWalletVoteSmallToken        macaroon.Token
	UserWalletVoteTestSmallSkuToken string

	AnonCardToken            macaroon.Token
	AnonCardVoteTestSkuToken string

	FreeTestToken    macaroon.Token
	FreeTestSkuToken string

	FreeTLTestToken    macaroon.Token
	FreeTLTestSkuToken string

	FreeTLTest1MToken    macaroon.Token
	FreeTLTest1MSkuToken string

	InvalidFreeTestSkuToken string
)

type ControllersTestSuite struct {
	service  *Service
	mockCB   *mockcb.MockClient
	mockCtrl *gomock.Controller
	storage  Datastore
	suite.Suite
	orderh *handler.Order
}

func TestControllersTestSuite(t *testing.T) {
	suite.Run(t, new(ControllersTestSuite))
}

func (suite *ControllersTestSuite) SetupSuite() {
	skustest.Migrate(suite.T())
	retryPolicy = retrypolicy.NoRetry // set this so we fail fast for cbr http requests
	govalidator.SetFieldsRequiredByDefault(true)

	storage, _ := NewPostgres(
		repository.NewOrder(),
		repository.NewOrderItem(),
		repository.NewOrderPayHistory(),
		repository.NewIssuer(),
		"", false, "",
	)

	suite.storage = storage

	AnonCardC := macaroon.Caveats{
		"sku":             "anon-card-vote",
		"description":     "brave anon-card-vote sku token v1",
		"credential_type": "single-use",
		"currency":        "BAT",
		"price":           "0.25",
	}

	UserWalletC := macaroon.Caveats{
		"sku":             "user-wallet-vote",
		"description":     "brave user-wallet-vote sku token v1",
		"credential_type": "single-use",
		"currency":        "BAT",
		"price":           "0.25",
	}

	UserWalletSmallC := macaroon.Caveats{
		"sku":             "user-wallet-vote",
		"description":     "brave user-wallet-vote sku token v1",
		"credential_type": "single-use",
		"currency":        "BAT",
		"price":           "0.00000000000000001",
	}

	FreeC := macaroon.Caveats{
		"sku":             "integration-test-free",
		"description":     "integration test free sku token",
		"credential_type": "single-use",
		"currency":        "BAT",
		"price":           "0.00",
	}

	FreeTLC := macaroon.Caveats{
		"sku":                       "integration-test-free",
		"description":               "integration test free sku token",
		"credential_type":           "time-limited",
		"credential_valid_duration": "P1M",
		"currency":                  "BAT",
		"price":                     "0.00",
	}

	FreeTL1MC := macaroon.Caveats{
		"sku":                       "integration-test-free-1m",
		"description":               "integration test free sku token",
		"credential_type":           "time-limited",
		"credential_valid_duration": "P1M",
		"issuance_interval":         "P1M",
		"currency":                  "BAT",
		"price":                     "0.00",
	}

	// create sku using key
	UserWalletVoteToken = macaroon.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macaroon.Caveats{UserWalletC},
	}

	UserWalletVoteSmallToken = macaroon.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macaroon.Caveats{UserWalletSmallC},
	}

	AnonCardToken = macaroon.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macaroon.Caveats{AnonCardC},
	}

	FreeTestToken = macaroon.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macaroon.Caveats{FreeC},
	}

	FreeTLTestToken = macaroon.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macaroon.Caveats{FreeTLC},
	}

	FreeTLTest1MToken = macaroon.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macaroon.Caveats{FreeTL1MC},
	}

	var err error
	// setup our global skus
	UserWalletVoteTestSkuToken, err = UserWalletVoteToken.Generate("testing123")
	suite.Require().NoError(err)

	// hacky, put this in development sku check
	skuMap["development"][UserWalletVoteTestSkuToken] = true

	UserWalletVoteTestSmallSkuToken, err = UserWalletVoteSmallToken.Generate("testing123")
	suite.Require().NoError(err)

	// hacky, put this in development sku check
	skuMap["development"][UserWalletVoteTestSmallSkuToken] = true

	AnonCardVoteTestSkuToken, err = AnonCardToken.Generate("testing123")
	suite.Require().NoError(err)

	// hacky, put this in development sku check
	skuMap["development"][AnonCardVoteTestSkuToken] = true

	FreeTestSkuToken, err = FreeTestToken.Generate("testing123")
	suite.Require().NoError(err)

	// hacky, put this in development sku check
	skuMap["development"][FreeTestSkuToken] = true

	FreeTLTestSkuToken, err = FreeTLTestToken.Generate("testing123")
	suite.Require().NoError(err)

	FreeTLTest1MSkuToken, err = FreeTLTest1MToken.Generate("testing123")
	suite.Require().NoError(err)

	// hacky, put this in development sku check
	skuMap["development"][FreeTLTestSkuToken] = true
	skuMap["development"][FreeTLTest1MSkuToken] = true

	// signed with wrong signing string
	InvalidFreeTestSkuToken, err = FreeTestToken.Generate("123testing")
	suite.Require().NoError(err)
}

func (suite *ControllersTestSuite) BeforeTest(sn, tn string) {
	pg, err := NewPostgres(
		repository.NewOrder(),
		repository.NewOrderItem(),
		repository.NewOrderPayHistory(),
		repository.NewIssuer(),
		"", false, "",
	)
	suite.Require().NoError(err, "Failed to get postgres conn")

	suite.mockCtrl = gomock.NewController(suite.T())
	suite.mockCB = mockcb.NewMockClient(suite.mockCtrl)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	EncryptionKey = "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0"
	InitEncryptionKeys()

	suite.service = &Service{
		issuerRepo: repository.NewIssuer(),
		Datastore:  pg,
		cbClient:   suite.mockCB,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
		retry: backoff.Retry,
	}

	suite.orderh = handler.NewOrder(suite.service)

	// encrypt merchant key
	cipher, nonce, err := cryptography.EncryptMessage(byteEncryptionKey, []byte("testing123"))
	suite.Require().NoError(err)

	// create key in db for our brave.com location
	_, err = suite.service.Datastore.CreateKey("brave.com", "brave.com", hex.EncodeToString(cipher), hex.EncodeToString(nonce[:]))
	suite.Require().NoError(err)
}

func (suite *ControllersTestSuite) AfterTest(sn, tn string) {
	skustest.CleanDB(suite.T(), suite.storage.RawDB())
	suite.mockCtrl.Finish()
}

func (suite *ControllersTestSuite) setupCreateOrder(skuToken string, token macaroon.Token, quantity int) (Order, *Issuer) {
	issuerID, err := encodeIssuerID(token.Location, token.FirstPartyCaveats[0]["sku"])
	suite.Require().NoError(err)

	// Mock out create issuer calls before we create the order.
	credType, ok := token.FirstPartyCaveats[0]["credential_type"]
	if ok && credType == singleUse {
		suite.mockCB.EXPECT().CreateIssuer(gomock.Any(), issuerID, gomock.Any()).Return(nil)

		resp := &cbr.IssuerResponse{
			Name:      issuerID,
			PublicKey: base64.StdEncoding.EncodeToString([]byte(test.RandomString())),
		}

		suite.mockCB.EXPECT().GetIssuer(gomock.Any(), gomock.Any()).Return(resp, nil)
	}

	// create order this will also create the issuer

	createRequest := &model.CreateOrderRequest{
		Items: []model.OrderItemRequest{
			{
				SKU:      skuToken,
				Quantity: quantity,
			},
		},
	}

	body, err := json.Marshal(&createRequest)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/v1/orders", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	req = req.WithContext(context.WithValue(req.Context(), appctx.EnvironmentCTXKey, "development"))

	rr := httptest.NewRecorder()

	handlers.AppHandler(suite.orderh.Create).ServeHTTP(rr, req)

	suite.Require().Equal(http.StatusCreated, rr.Code)

	var order Order
	{
		err := json.Unmarshal(rr.Body.Bytes(), &order)
		suite.Require().NoError(err)
	}

	repo := repository.NewIssuer()

	var exp error
	if credType == timeLimited {
		exp = model.ErrIssuerNotFound
	}

	issuer, err := repo.GetByMerchID(context.TODO(), suite.storage.RawDB(), issuerID)
	suite.Require().Equal(exp, err)

	return order, issuer
}

func (suite *ControllersTestSuite) TestIOSWebhookCertFail() {
	order, _ := suite.setupCreateOrder(UserWalletVoteTestSkuToken, UserWalletVoteToken, 40)
	suite.Assert().NotNil(order)

	// Check the order
	suite.Assert().Equal("10", order.TotalPrice.String())

	// add the external id to metadata as if an initial receipt was submitted
	err := suite.service.Datastore.AppendOrderMetadata(context.Background(), &order.ID, "externalID", "my external id")
	suite.Require().NoError(err)

	handler := HandleIOSWebhook(suite.service)

	// create a jws message to send
	body := []byte{}

	// create request to webhook
	req, err := http.NewRequest("POST", "/v1/ios", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	req = req.WithContext(context.WithValue(req.Context(), appctx.EnvironmentCTXKey, "development"))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Require().Equal(http.StatusBadRequest, rr.Code)
}

func (suite *ControllersTestSuite) TestAndroidWebhook() {
	order, _ := suite.setupCreateOrder(UserWalletVoteTestSkuToken, UserWalletVoteToken, 40)
	suite.Assert().NotNil(order)

	// Check the order
	suite.Assert().Equal("10", order.TotalPrice.String())

	// add the external id to metadata as if an initial receipt was submitted
	err := suite.storage.AppendOrderMetadata(context.Background(), &order.ID, "externalID", "my external id")
	suite.Require().NoError(err)

	suite.service.vendorReceiptValid = &mockVendorReceiptValidator{
		fnValidateGoogle: func(ctx context.Context, receipt SubmitReceiptRequestV1) (string, error) {
			return "my external id", nil
		},
	}

	suite.service.gcpValidator = &mockGcpRequestValidator{}

	handler := HandleAndroidWebhook(suite.service)

	// notification message
	devNotify := DeveloperNotification{
		PackageName: "package name",
		SubscriptionNotification: SubscriptionNotification{
			NotificationType: androidSubscriptionCanceled,
			PurchaseToken:    "my external id",
			SubscriptionID:   "subscription id",
		},
	}

	buf, err := json.Marshal(&devNotify)
	suite.Require().NoError(err)

	// wrapper notification message
	notification := &AndroidNotification{
		Message: AndroidNotificationMessage{
			Data: base64.StdEncoding.EncodeToString(buf), // dev notification is b64 encoded
		},
		Subscription: "subscription",
	}

	body, err := json.Marshal(&notification)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/v1/android", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	req = req.WithContext(context.WithValue(req.Context(), appctx.EnvironmentCTXKey, "development"))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Require().Equal(http.StatusOK, rr.Code)

	// get order and check the state changed to canceled
	updatedOrder, err := suite.service.Datastore.GetOrder(order.ID)
	suite.Assert().Equal("canceled", updatedOrder.Status)
}

func (suite *ControllersTestSuite) TestCreateOrder() {
	order, _ := suite.setupCreateOrder(UserWalletVoteTestSkuToken, UserWalletVoteToken, 40)

	// Check the order
	suite.Assert().Equal("10", order.TotalPrice.String())
	suite.Assert().Equal("pending", order.Status)
	suite.Assert().Equal("BAT", order.Currency)

	// Check the order items
	suite.Assert().Equal(len(order.Items), 1)
	suite.Assert().Equal("BAT", order.Items[0].Currency)
	suite.Assert().Equal("0.25", order.Items[0].Price.String())
	suite.Assert().Equal(40, order.Items[0].Quantity)
	suite.Assert().Equal(decimal.New(10, 0), order.Items[0].Subtotal)
	suite.Assert().Equal(order.ID, order.Items[0].OrderID)
	suite.Assert().Equal("user-wallet-vote", order.Items[0].SKU)
}

func (suite *ControllersTestSuite) TestCreateFreeOrderWhitelistedSKU() {
	order, _ := suite.setupCreateOrder(FreeTestSkuToken, FreeTestToken, 10)

	// Check the order
	suite.Assert().Equal("0", order.TotalPrice.String())
	suite.Assert().Equal("paid", order.Status)
	suite.Assert().Equal("BAT", order.Currency)

	// Check the order items
	suite.Assert().Equal(len(order.Items), 1)
	suite.Assert().Equal("BAT", order.Items[0].Currency)
	suite.Assert().Equal("0", order.Items[0].Price.String())
	suite.Assert().Equal(10, order.Items[0].Quantity)
	suite.Assert().Equal(decimal.New(0, 0), order.Items[0].Subtotal)
	suite.Assert().Equal(order.ID, order.Items[0].OrderID)
	suite.Assert().Equal("integration-test-free", order.Items[0].SKU)
}

func (suite *ControllersTestSuite) TestCreateInvalidOrder() {
	createRequest := &model.CreateOrderRequest{
		Items: []model.OrderItemRequest{
			{
				SKU:      InvalidFreeTestSkuToken,
				Quantity: 1,
			},
		},
	}
	body, err := json.Marshal(&createRequest)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/v1/orders", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	req = req.WithContext(context.WithValue(req.Context(), appctx.EnvironmentCTXKey, "development"))

	rr := httptest.NewRecorder()

	handlers.AppHandler(suite.orderh.Create).ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)

	suite.Require().Contains(rr.Body.String(), "Invalid SKU Token provided in request")
}

func (suite *ControllersTestSuite) TestGetOrder() {
	order, _ := suite.setupCreateOrder(UserWalletVoteTestSkuToken, UserWalletVoteToken, 20)

	req, err := http.NewRequest("GET", "/v1/orders/{orderID}", nil)
	suite.Require().NoError(err)

	getOrderHandler := GetOrder(suite.service)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	getReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	getOrderHandler.ServeHTTP(rr, getReq)
	suite.Require().Equal(http.StatusOK, rr.Code)

	err = json.Unmarshal(rr.Body.Bytes(), &order)
	suite.Require().NoError(err)

	suite.Assert().Equal("5", order.TotalPrice.String())
	suite.Assert().Equal("pending", order.Status)

	// Check the order items
	suite.Assert().Equal(len(order.Items), 1)
	suite.Assert().Equal("BAT", order.Items[0].Currency)
	suite.Assert().Equal("0.25", order.Items[0].Price.String())
	suite.Assert().Equal(20, order.Items[0].Quantity)
	suite.Assert().Equal(decimal.New(5, 0), order.Items[0].Subtotal)
	suite.Assert().Equal(order.ID, order.Items[0].OrderID)
}

func (suite *ControllersTestSuite) TestGetMissingOrder() {
	req, err := http.NewRequest("GET", "/v1/orders/{orderID}", nil)
	suite.Require().NoError(err)

	getOrderHandler := GetOrder(suite.service)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("orderID", "9645ca16-bc93-4e37-8edf-cb35b1763216")
	getReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	getOrderHandler.ServeHTTP(rr, getReq)
	suite.Assert().Equal(http.StatusNotFound, rr.Code)
}

func (suite *ControllersTestSuite) TestE2EOrdersGeminiTransactions() {
	pg, err := NewPostgres(
		repository.NewOrder(),
		repository.NewOrderItem(),
		repository.NewOrderPayHistory(),
		repository.NewIssuer(),
		"", false, "",
	)
	suite.Require().NoError(err, "Failed to get postgres conn")

	service := &Service{
		Datastore: pg,
	}
	order, _ := suite.setupCreateOrder(UserWalletVoteTestSkuToken, UserWalletVoteToken, 1/.25)

	handler := CreateGeminiTransaction(service)

	createRequest := &CreateTransactionRequest{
		ExternalTransactionID: "150d7a21-c203-4ba4-8fdf-c5fc36aca004",
	}

	body, err := json.Marshal(&createRequest)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/v1/orders/{orderID}/transactions/gemini", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	postReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// setup fake gemini client
	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()
	mockGemini := mockgemini.NewMockClient(mockCtrl)

	settlementAddress := "settlement"
	currency := "BAT"
	status := "completed"
	amount, err := decimal.NewFromString("0.0000000001")
	suite.Require().NoError(err)
	// make sure we get a call to CheckTxStatus and return the right things
	mockGemini.EXPECT().
		CheckTxStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(
			&gemini.PayoutResult{
				Destination: &settlementAddress,
				Amount:      &amount,
				Currency:    &currency,
				Status:      &status,
			}, nil)

	// setup context and client
	service.geminiClient = mockGemini
	service.geminiConf = &gemini.Conf{
		APIKey:            "key",
		Secret:            "secret",
		ClientID:          "client_id",
		SettlementAddress: settlementAddress,
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, postReq)

	suite.Require().Equal(http.StatusCreated, rr.Code)

	var transaction Transaction
	err = json.Unmarshal(rr.Body.Bytes(), &transaction)
	suite.Require().NoError(err)

	// Check the transaction
	suite.Assert().Equal(amount, transaction.Amount)
	suite.Assert().Equal("gemini", transaction.Kind)
	suite.Assert().Equal("completed", transaction.Status)
	suite.Assert().Equal("BAT", transaction.Currency)
	suite.Assert().Equal(createRequest.ExternalTransactionID, transaction.ExternalTransactionID)
	suite.Assert().Equal(order.ID, transaction.OrderID, order.TotalPrice)

	// Check the order was updated to paid
	// Old order
	suite.Assert().Equal("pending", order.Status)
	// Check the new order

	// this is not possible to test end to end, settlement bots are out of our control
	// and sometimes take upwards of 10 minutes.  Only reason this worked before was
	// we had asked them to make them run quicker...  Not sure this is a good test
	// FIXME: figure out how we can do this without waiting for their settlement bots
	//updatedOrder, err := service.Datastore.GetOrder(order.ID)
	//suite.Require().NoError(err)
	//suite.Assert().Equal("paid", updatedOrder.Status)

	// make sure we get a call to CheckTxStatus and return the right things
	mockGemini.EXPECT().
		CheckTxStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(
			&gemini.PayoutResult{
				Destination: &settlementAddress,
				Amount:      &amount,
				Currency:    &currency,
				Status:      &status,
			}, nil)

	req, err = http.NewRequest("POST", "/v1/orders/{orderID}/transactions/gemini", bytes.NewBuffer(body))

	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	postReq = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// setup context and client
	service.geminiClient = mockGemini
	service.geminiConf = &gemini.Conf{
		APIKey:            "key",
		Secret:            "secret",
		ClientID:          "client_id",
		SettlementAddress: settlementAddress,
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, postReq)
	// now should be a 200 for updating tx
	suite.Require().Equal(http.StatusOK, rr.Code)
}

func (suite *ControllersTestSuite) TestE2EOrdersUpholdTransactions() {
	orderAmount := decimal.NewFromFloat(0.00000000000000001)

	order, _ := suite.setupCreateOrder(UserWalletVoteTestSmallSkuToken, UserWalletVoteToken, 1)

	handler := CreateUpholdTransaction(suite.service)

	// create an uphold wallet
	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err, "Failed to create wallet keypair")

	walletID := uuid.NewV4()
	bat := altcurrency.BAT
	info := walletutils.Info{
		ID:          walletID.String(),
		Provider:    "uphold",
		ProviderID:  "-",
		AltCurrency: &bat,
		PublicKey:   hex.EncodeToString(publicKey),
		LastBalance: nil,
	}

	ctx := context.Background()
	// setup debug for client
	ctx = context.WithValue(ctx, appctx.DebugLoggingCTXKey, true)
	// setup debug log level
	ctx = context.WithValue(ctx, appctx.LogLevelCTXKey, "debug")

	// setup a new logger, add to context as well
	_, logger := logutils.SetupLogger(ctx)

	w := uphold.Wallet{
		Logger:  logger,
		Info:    info,
		PrivKey: privKey,
		PubKey:  publicKey,
	}
	err = w.Register(ctx, "drain-card-test")
	suite.Require().NoError(err, "Failed to register wallet")

	_, err = uphold.FundWallet(ctx, &w, altcurrency.BAT.ToProbi(orderAmount))
	suite.Require().NoError(err, "Failed to fund wallet")

	<-time.After(1 * time.Second)

	// pay the transaction
	settlementAddr := os.Getenv("BAT_SETTLEMENT_ADDRESS")
	tInfo, err := w.Transfer(ctx, altcurrency.BAT, altcurrency.BAT.ToProbi(orderAmount), settlementAddr)
	suite.Require().NoError(err)

	createRequest := &CreateTransactionRequest{
		ExternalTransactionID: tInfo.ID,
	}

	body, err := json.Marshal(&createRequest)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/v1/orders/{orderID}/transactions/uphold", bytes.NewBuffer(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	postReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	suite.Require().NoError(err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, postReq)

	suite.Require().Equal(http.StatusCreated, rr.Code)

	var transaction Transaction
	err = json.Unmarshal(rr.Body.Bytes(), &transaction)
	suite.Require().NoError(err)

	// Check the transaction
	suite.Assert().Equal(orderAmount, transaction.Amount)
	suite.Assert().Equal("uphold", transaction.Kind)
	suite.Assert().Equal("completed", transaction.Status)
	suite.Assert().Equal("BAT", transaction.Currency)
	suite.Assert().Equal(createRequest.ExternalTransactionID, transaction.ExternalTransactionID)
	suite.Assert().Equal(order.ID, transaction.OrderID, order.TotalPrice)

	// Check the order was updated to paid
	// Old order
	suite.Assert().Equal("pending", order.Status)
	// Check the new order
	updatedOrder, err := suite.service.Datastore.GetOrder(order.ID)
	suite.Require().NoError(err)
	suite.Assert().Equal("paid", updatedOrder.Status)

	// Test to make sure on repost we update tx
	req, err = http.NewRequest("POST", "/v1/orders/{orderID}/transactions/uphold", bytes.NewBuffer(body))

	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	postReq = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	suite.Require().NoError(err)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, postReq)

	// now it should be a 200 when updating a tx status
	suite.Require().Equal(http.StatusOK, rr.Code)
}

func (suite *ControllersTestSuite) TestGetTransactions() {
	// External transaction has 12 BAT
	order, _ := suite.setupCreateOrder(UserWalletVoteTestSkuToken, UserWalletVoteToken, 12/.25)

	handler := CreateUpholdTransaction(suite.service)

	createRequest := &CreateTransactionRequest{
		ExternalTransactionID: "9d5b6a7d-795b-4f02-a91e-25eee2852ebf",
	}

	body, err := json.Marshal(&createRequest)
	suite.Require().NoError(err)

	oldUpholdSettlementAddress := uphold.UpholdSettlementAddress
	uphold.UpholdSettlementAddress = "6654ecb0-6079-4f6c-ba58-791cc890a561"

	defer func() {
		uphold.UpholdSettlementAddress = oldUpholdSettlementAddress
	}()

	req, err := http.NewRequest("POST", "/v1/orders/{orderID}/transactions/uphold", bytes.NewBuffer(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	postReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	suite.Require().NoError(err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, postReq)

	suite.Require().Equal(http.StatusCreated, rr.Code)

	var transaction Transaction
	err = json.Unmarshal(rr.Body.Bytes(), &transaction)
	suite.Require().NoError(err)

	// Check the transaction
	suite.Assert().Equal(decimal.NewFromFloat32(12), transaction.Amount)
	suite.Assert().Equal("uphold", transaction.Kind)
	suite.Assert().Equal("completed", transaction.Status)
	suite.Assert().Equal("BAT", transaction.Currency)
	suite.Assert().Equal(createRequest.ExternalTransactionID, transaction.ExternalTransactionID)
	suite.Assert().Equal(order.ID, transaction.OrderID)

	// Check the order was updated to paid
	// Old order
	suite.Assert().Equal("pending", order.Status)
	// Check the new order
	updatedOrder, err := suite.service.Datastore.GetOrder(order.ID)
	suite.Require().NoError(err)
	suite.Assert().Equal("paid", updatedOrder.Status)

	// Get all the transactions, should only be one

	handler = GetTransactions(suite.service)
	req, err = http.NewRequest("GET", "/v1/orders/{orderID}/transactions", nil)
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	getReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	suite.Require().NoError(err)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, getReq)

	suite.Require().Equal(http.StatusOK, rr.Code)
	var transactions []Transaction
	err = json.Unmarshal(rr.Body.Bytes(), &transactions)
	suite.Require().NoError(err)

	// Check the transaction
	suite.Assert().Equal(decimal.NewFromFloat32(12), transactions[0].Amount)
	suite.Assert().Equal("uphold", transactions[0].Kind)
	suite.Assert().Equal("completed", transactions[0].Status)
	suite.Assert().Equal("BAT", transactions[0].Currency)
	suite.Assert().Equal(createRequest.ExternalTransactionID, transactions[0].ExternalTransactionID)
	suite.Assert().Equal(order.ID, transactions[0].OrderID)
}

func generateWallet(ctx context.Context, t *testing.T) *uphold.Wallet {
	var info walletutils.Info
	info.ID = uuid.NewV4().String()
	info.Provider = "uphold"
	info.ProviderID = ""
	{
		tmp := altcurrency.BAT
		info.AltCurrency = &tmp
	}

	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	info.PublicKey = hex.EncodeToString(publicKey)
	newWallet := &uphold.Wallet{Info: info, PrivKey: privateKey, PubKey: publicKey}
	err = newWallet.Register(ctx, "bat-go test card")
	if err != nil {
		t.Fatal(err)
	}
	return newWallet
}

func (suite *ControllersTestSuite) fetchTimeLimitedCredentials(service *Service, order Order) (ordercreds []TimeLimitedCreds) {

	o, err := suite.service.Datastore.GetOrder(order.ID)
	var ii string
	if o.Items == nil || o.Items[0].IssuanceIntervalISO == nil {
		ii = "P1D"
	} else {
		ii = *(o.Items[0].IssuanceIntervalISO)
	}

	// Check to see if we have HTTP Accepted
	handler := GetOrderCreds(service)
	req, err := http.NewRequest("GET", "/{orderID}/credentials", nil)
	suite.Require().NoError(err)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Assert().Equal(http.StatusOK, rr.Code)

	// see if we can get our order creds
	handler = GetOrderCreds(service)
	req, err = http.NewRequest("GET", "/{orderID}/credentials", nil)
	suite.Require().NoError(err)

	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)

	err = json.Unmarshal([]byte(rr.Body.String()), &ordercreds)
	suite.Require().NoError(err)

	isoD, err := timeutils.ParseDuration("P1M") // 1 month from the sku
	suite.Require().NoError(err)

	ft, err := isoD.FromNow()
	suite.Require().NoError(err)

	validFor := time.Until(*ft)

	suite.Require().NoError(err)

	// get the go duration of our issuance interval
	interval, err := timeutils.ParseDuration(ii)
	suite.Require().NoError(err)
	chunk, err := interval.FromNow()
	suite.Require().NoError(err)

	issuanceInterval := time.Until(*chunk)

	// validate we get the right number of creds back, 1 per day
	numTokens := int(validFor / issuanceInterval)
	suite.Require().True(numTokens <= len(ordercreds), "should have a buffer of tokens")

	return
}

func (suite *ControllersTestSuite) fetchCredentials(ctx context.Context, server *http.Server, order Order, issuer Issuer) (signature, tokenPreimage string, ordercreds []OrderCreds) {
	signature = "PsavkSWaqsTzZjmoDBmSu6YxQ7NZVrs2G8DQ+LkW5xOejRF6whTiuUJhr9dJ1KlA+79MDbFeex38X5KlnLzvJw=="
	tokenPreimage = "125KIuuwtHGEl35cb5q1OLSVepoDTgxfsvwTc7chSYUM2Zr80COP19EuMpRQFju1YISHlnB04XJzZYN2ieT9Ng=="

	// perform create order creds request
	credsReq := CreateOrderCredsRequest{
		ItemID:       order.Items[0].ID,
		BlindedCreds: []string{base64.StdEncoding.EncodeToString([]byte(test.RandomString()))},
	}
	body, err := json.Marshal(credsReq)
	suite.Require().NoError(err)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/%s/credentials", order.ID),
		bytes.NewBuffer(body)).WithContext(ctx)
	rr := httptest.NewRecorder()

	server.Handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)

	// check to see if order creds are waiting to be processed.
	// We can expect a http accepted status when a job is submitted but not processed.
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/%s/credentials", order.ID), nil)
	rr = httptest.NewRecorder()

	server.Handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusAccepted, rr.Code)

	// The CreateOrderCreds request writes to the db table signing_order_request_outbox which gets sent to Kafka
	// and CBR for signing. To mock that insert the Kafka result.

	to := time.Now().Add(time.Hour).Format(time.RFC3339)
	from := time.Now().Local().Format(time.RFC3339)

	metadata := Metadata{
		ItemID:         order.Items[0].ID,
		OrderID:        order.ID,
		IssuerID:       issuer.ID,
		CredentialType: order.Items[0].CredentialType,
	}

	associatedData, err := json.Marshal(metadata)
	suite.Require().NoError(err)

	signingOrderResult := SigningOrderResult{
		RequestID: uuid.NewV4().String(),
		Data: []SignedOrder{
			{
				PublicKey:      issuer.PublicKey,
				Proof:          test.RandomString(),
				Status:         SignedOrderStatusOk,
				BlindedTokens:  credsReq.BlindedCreds,
				SignedTokens:   []string{test.RandomString()},
				ValidTo:        &UnionNullString{"string": to},
				ValidFrom:      &UnionNullString{"string": from},
				AssociatedData: associatedData,
			},
		},
	}

	_, tx, _, commit, err := datastore.GetTx(ctx, suite.storage)

	err = suite.storage.InsertSignedOrderCredentialsTx(ctx, tx, &signingOrderResult)
	suite.Require().NoError(err)

	err = commit()
	suite.Require().NoError(err)

	// get the signed order credentials
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/%s/credentials", order.ID), nil)

	rr = httptest.NewRecorder()

	server.Handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)

	err = json.NewDecoder(rr.Body).Decode(&ordercreds)
	suite.Require().NoError(err)

	return
}

func (suite *ControllersTestSuite) TestE2EAnonymousCard() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	voteTopic = test.RandomString()
	kafkaSignedOrderCredsTopic = test.RandomString()
	kafkaSignedOrderCredsDLQTopic = os.Getenv("GRANT_CBP_SIGN_CONSUMER_TOPIC_DLQ")
	kafkaSignedRequestReaderGroupID = test.RandomString()

	ctx = skustest.SetupKafka(ctx, suite.T(), voteTopic,
		kafkaUnsignedOrderCredsTopic, kafkaSignedOrderCredsTopic, kafkaSignedOrderCredsDLQTopic)

	err := suite.service.InitKafka(ctx)
	suite.Require().NoError(err)

	authMwr := NewAuthMwr(suite.service)
	instrumentHandler := func(name string, h http.Handler) http.Handler {
		return h
	}

	router := Router(suite.service, authMwr, instrumentHandler, newCORSOptsEnv())

	router.Mount("/vote", VoteRouter(suite.service, instrumentHandler))
	server := &http.Server{Addr: ":8080", Handler: router}

	// NextVoteDrainJob monitors vote queue
	go func() {
		for {
			select {
			case <-ctx.Done():
				break
			default:
				_, _ = suite.service.RunNextVoteDrainJob(ctx)
				<-time.After(50 * time.Millisecond)
			}
		}
	}()

	numVotes := 1
	order, issuer := suite.setupCreateOrder(AnonCardVoteTestSkuToken, AnonCardToken, numVotes)

	userWallet := generateWallet(ctx, suite.T())
	err = suite.service.wallet.Datastore.UpsertWallet(ctx, &userWallet.Info)
	suite.Require().NoError(err)

	balanceBefore, err := userWallet.GetBalance(ctx, true)
	suite.Require().NoError(err)
	balanceAfter, err := uphold.FundWallet(ctx, userWallet, order.TotalPrice)
	suite.Require().NoError(err)

	// wait for balance to become available
	for i := 0; i < 5; i++ {
		select {
		case <-time.After(500 * time.Millisecond):
			balances, err := userWallet.GetBalance(ctx, true)
			suite.Require().NoError(err)
			totalProbi := altcurrency.BAT.FromProbi(balances.TotalProbi)
			if totalProbi.GreaterThan(decimal.Zero) {
				break
			}
		}
	}

	suite.Require().True(balanceAfter.GreaterThan(balanceBefore.TotalProbi), "balance should have increased")
	txn, err := userWallet.PrepareTransaction(altcurrency.BAT, altcurrency.BAT.ToProbi(order.TotalPrice),
		uphold.SettlementDestination, "bat-go:grant-server.TestAC", "", nil)
	suite.Require().NoError(err)

	walletID, err := uuid.FromString(userWallet.ID)
	suite.Require().NoError(err)

	// create anonymous card
	anonCardRequest := CreateAnonCardTransactionRequest{
		WalletID:    walletID,
		Transaction: txn,
	}
	body, err := json.Marshal(anonCardRequest)
	suite.Require().NoError(err)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/%s/transactions/anonymousCard",
		order.ID), bytes.NewBuffer(body))
	suite.Require().NoError(err)

	rr := httptest.NewRecorder()

	server.Handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusCreated, rr.Code)

	signature, tokenPreimage, orderCreds := suite.fetchCredentials(ctx, server, order, *issuer)

	suite.Require().Equal(len(*(*[]string)(orderCreds[0].SignedCreds)), order.Items[0].Quantity)

	// Check we can retrieve the order by order and item id
	r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/%s/credentials/%s",
		order.ID, order.Items[0].ID), nil)

	rw := httptest.NewRecorder()

	server.Handler.ServeHTTP(rw, r)

	suite.Require().Equal(http.StatusOK, rw.Code)

	// setup vote
	vote := Vote{
		Type:    "auto-contribute",
		Channel: "brave.com",
	}
	voteBytes, err := json.Marshal(vote)
	suite.Require().NoError(err)

	votePayload := base64.StdEncoding.EncodeToString(voteBytes)

	suite.mockCB.EXPECT().RedeemCredentials(gomock.Any(),
		[]cbr.CredentialRedemption{
			{
				Issuer:        issuer.Name(),
				TokenPreimage: tokenPreimage,
				Signature:     signature,
			},
		}, votePayload).Return(nil)

	// perform create vote request
	voteReq := VoteRequest{
		Vote: votePayload,
		Credentials: []CredentialBinding{{
			PublicKey:     *orderCreds[0].PublicKey,
			TokenPreimage: tokenPreimage,
			Signature:     signature,
		}},
	}

	body, err = json.Marshal(voteReq)
	suite.Require().NoError(err)

	req = httptest.NewRequest(http.MethodPost, "/vote", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	rr = httptest.NewRecorder()

	server.Handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)

	<-time.After(5 * time.Second)

	voteEvent := suite.ReadKafkaVoteEvent(ctx)

	suite.Assert().Equal(vote.Type, voteEvent.Type)
	suite.Assert().Equal(vote.Channel, voteEvent.Channel)
	// should be number of credentials for the vote
	suite.Assert().Equal(int64(len(voteReq.Credentials)), voteEvent.VoteTally)
	// check that the funding source matches the issuer
	suite.Assert().Equal("anonymous-card", voteEvent.FundingSource) // from SKU...
}

func (suite *ControllersTestSuite) TestTimeLimitedCredentialsVerifyPresentation1M() {
	order, _ := suite.setupCreateOrder(FreeTLTest1MSkuToken, FreeTLTest1MToken, 1)

	// setup a key for our merchant
	k := suite.SetupCreateKey(order.MerchantID)
	// GetKey
	keyID, err := uuid.FromString(k.ID)
	suite.Require().NoError(err, "error parsing key id")

	key, err := suite.service.Datastore.GetKey(keyID, false)
	suite.Require().NoError(err, "error getting key from db")
	secret, err := key.GetSecretKey()
	suite.Require().NoError(err, "unable to decrypt secret")

	ordercreds := suite.fetchTimeLimitedCredentials(suite.service, order)

	issuerID, err := encodeIssuerID(order.MerchantID, "integration-test-free-1m")
	suite.Require().NoError(err, "error attempting to encode issuer id")

	timeLimitedSecret := cryptography.NewTimeLimitedSecret([]byte(*secret))
	for _, cred := range ordercreds {
		issued, err := time.Parse("2006-01-02", cred.IssuedAt)
		suite.Require().NoError(err, "error attempting to parse issued at")
		expires, err := time.Parse("2006-01-02", cred.ExpiresAt)
		suite.Require().NoError(err, "error attempting to parse expires at")

		ok, err := timeLimitedSecret.Verify([]byte(issuerID), issued, expires, cred.Token)
		suite.Require().NoError(err, "error attempting to verify time limited cred")
		suite.Require().True(ok, "verify failed")
	}

	var (
		lastIssued  time.Time
		lastExpired time.Time
	)

	var first = true
	for _, cred := range ordercreds {
		issued, err := time.Parse("2006-01-02", cred.IssuedAt)
		suite.Require().NoError(err, "error attempting to parse issued at")
		expires, err := time.Parse("2006-01-02", cred.ExpiresAt)
		suite.Require().NoError(err, "error attempting to parse expires at")

		if !first {
			// sometimes the first month of empty time is 1
			// validate each cred is for a different month
			suite.Require().True(issued.Month() != lastIssued.Month())
			suite.Require().True(expires.Month() != lastExpired.Month())
		}
		first = false

		lastIssued = issued
		lastExpired = expires
	}
}

func (suite *ControllersTestSuite) TestTimeLimitedCredentialsVerifyPresentation() {
	order, _ := suite.setupCreateOrder(FreeTLTestSkuToken, FreeTLTestToken, 1)

	// setup a key for our merchant
	k := suite.SetupCreateKey(order.MerchantID)
	// GetKey
	keyID, err := uuid.FromString(k.ID)
	suite.Require().NoError(err, "error parsing key id")

	key, err := suite.service.Datastore.GetKey(keyID, false)
	suite.Require().NoError(err, "error getting key from db")
	secret, err := key.GetSecretKey()
	suite.Require().NoError(err, "unable to decrypt secret")

	ordercreds := suite.fetchTimeLimitedCredentials(suite.service, order)

	issuerID, err := encodeIssuerID(order.MerchantID, "integration-test-free")
	suite.Require().NoError(err, "error attempting to encode issuer id")

	timeLimitedSecret := cryptography.NewTimeLimitedSecret([]byte(*secret))
	for _, cred := range ordercreds {
		issued, err := time.Parse("2006-01-02", cred.IssuedAt)
		suite.Require().NoError(err, "error attempting to parse issued at")
		expires, err := time.Parse("2006-01-02", cred.ExpiresAt)
		suite.Require().NoError(err, "error attempting to parse expires at")

		ok, err := timeLimitedSecret.Verify([]byte(issuerID), issued, expires, cred.Token)
		suite.Require().NoError(err, "error attempting to verify time limited cred")
		suite.Require().True(ok, "verify failed")
	}

	var (
		lastIssued  time.Time
		lastExpired time.Time
	)

	var first = true
	for _, cred := range ordercreds {
		issued, err := time.Parse("2006-01-02", cred.IssuedAt)
		suite.Require().NoError(err, "error attempting to parse issued at")
		expires, err := time.Parse("2006-01-02", cred.ExpiresAt)
		suite.Require().NoError(err, "error attempting to parse expires at")

		if !first {
			// sometimes the first day of empty time is 1
			// validate each cred is for a different day
			suite.Require().True(issued.Day() != lastIssued.Day())
			suite.Require().True(expires.Day() != lastExpired.Day())
		}
		first = false

		lastIssued = issued
		lastExpired = expires
	}
}

/* func (suite *ControllersTestSuite) TestFailureToSignCredentialsBadPoint()
This test is no longer valid, as we no longer are using the HTTP endpoint for signing requests,
but rather using kafka for signing request/results.  No longer an applicable test.  Leaving note here
as tombstone.

*/

func (suite *ControllersTestSuite) SetupCreateKey(merchantID string) Key {
	createRequest := &CreateKeyRequest{
		Name: "BAT-GO",
	}

	body, err := json.Marshal(&createRequest)
	suite.Require().NoError(err)
	req, err := http.NewRequest("POST", "/v1/merchants/{merchantID}/key", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	createAPIHandler := CreateKey(suite.service)
	rctx := chi.NewRouteContext()
	//rctx.URLParams.Add("merchantID", "48dc25ed-4121-44ef-8147-4416a76201f7")
	rctx.URLParams.Add("merchantID", merchantID)
	postReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	createAPIHandler.ServeHTTP(rr, postReq)

	suite.Assert().Equal(http.StatusOK, rr.Code)

	var key Key
	err = json.Unmarshal(rr.Body.Bytes(), &key)
	suite.Assert().NoError(err)

	return key
}

func (suite *ControllersTestSuite) SetupDeleteKey(key Key) Key {
	deleteRequest := &DeleteKeyRequest{
		DelaySeconds: 0,
	}

	body, err := json.Marshal(&deleteRequest)
	suite.Require().NoError(err)

	req, err := http.NewRequest("DELETE", "/v1/merchants/id/key/{id}", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	deleteAPIHandler := DeleteKey(suite.service)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", key.ID)
	deleteReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	deleteAPIHandler.ServeHTTP(rr, deleteReq)
	suite.Assert().Equal(http.StatusOK, rr.Code)

	var deletedKey Key
	err = json.Unmarshal(rr.Body.Bytes(), &deletedKey)
	suite.Assert().NoError(err)

	return deletedKey
}

func (suite *ControllersTestSuite) TestCreateKey() {
	Key := suite.SetupCreateKey("48dc25ed-4121-44ef-8147-4416a76201f7")
	suite.Assert().Equal("48dc25ed-4121-44ef-8147-4416a76201f7", Key.Merchant)
}

func (suite *ControllersTestSuite) TestDeleteKey() {
	key := suite.SetupCreateKey("48dc25ed-4121-44ef-8147-4416a76201f7")

	deleteTime := time.Now()
	deletedKey := suite.SetupDeleteKey(key)
	// Ensure the expiry is within 5 seconds of when we made the call
	suite.Assert().WithinDuration(deleteTime, *deletedKey.Expiry, 5*time.Second)
}

func (suite *ControllersTestSuite) TestGetKeys() {
	pg, err := NewPostgres(
		repository.NewOrder(),
		repository.NewOrderItem(),
		repository.NewOrderPayHistory(),
		repository.NewIssuer(),
		"", false, "",
	)
	suite.Require().NoError(err, "Failed to get postgres conn")

	// Delete transactions so we don't run into any validation errors
	_, err = pg.RawDB().Exec("DELETE FROM api_keys;")
	suite.Require().NoError(err)

	key := suite.SetupCreateKey("48dc25ed-4121-44ef-8147-4416a76201f7")

	req, err := http.NewRequest("GET", "/v1/merchant/{merchantID}/keys", nil)
	suite.Require().NoError(err)

	getAPIHandler := GetKeys(suite.service)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("merchantID", key.Merchant)
	getReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	getAPIHandler.ServeHTTP(rr, getReq)

	suite.Assert().Equal(http.StatusOK, rr.Code)

	var keys []Key
	err = json.Unmarshal(rr.Body.Bytes(), &keys)
	suite.Assert().NoError(err)

	suite.Assert().Equal(1, len(keys))
}

func (suite *ControllersTestSuite) TestGetKeysFiltered() {
	pg, err := NewPostgres(
		repository.NewOrder(),
		repository.NewOrderItem(),
		repository.NewOrderPayHistory(),
		repository.NewIssuer(),
		"", false, "",
	)
	suite.Require().NoError(err, "Failed to get postgres conn")

	// Delete transactions so we don't run into any validation errors
	_, err = pg.RawDB().Exec("DELETE FROM api_keys;")
	suite.Require().NoError(err)

	key := suite.SetupCreateKey("48dc25ed-4121-44ef-8147-4416a76201f7")
	toDelete := suite.SetupCreateKey("48dc25ed-4121-44ef-8147-4416a76201f7")
	suite.SetupDeleteKey(toDelete)

	req, err := http.NewRequest("GET", "/v1/merchant/{merchantID}/keys?expired=true", nil)
	suite.Require().NoError(err)

	getAPIHandler := GetKeys(suite.service)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("merchantID", key.Merchant)
	getReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	getAPIHandler.ServeHTTP(rr, getReq)

	suite.Assert().Equal(http.StatusOK, rr.Code)

	var keys []Key
	err = json.Unmarshal(rr.Body.Bytes(), &keys)
	suite.Assert().NoError(err)

	suite.Assert().Equal(2, len(keys))
}

func (suite *ControllersTestSuite) TestExpiredTimeLimitedCred() {
	ctx := context.Background()
	valid := 1 * time.Second
	lastPaid := time.Now().Add(-1 * time.Minute)
	expiresAt := lastPaid.Add(valid)

	order := &Order{
		Location: datastore.NullString{
			NullString: sql.NullString{
				Valid: true, String: "brave.com",
			},
		},
		Status: OrderStatusPaid, LastPaidAt: &lastPaid,
		ExpiresAt: &expiresAt,
		ValidFor:  &valid,
	}

	creds, status, err := suite.service.GetTimeLimitedCreds(ctx, order, uuid.Nil, uuid.Nil)
	suite.Require().True(creds == nil, "should not get creds back")
	suite.Require().True(status == http.StatusBadRequest, "should not get creds back")
	suite.Require().Error(err, "should get an error")
}

func (suite *ControllersTestSuite) ReadKafkaVoteEvent(ctx context.Context) VoteEvent {
	kafkaVoteReader, err := kafkautils.NewKafkaReader(ctx, test.RandomString(), voteTopic)
	suite.Require().NoError(err)

	msg, err := kafkaVoteReader.ReadMessage(context.Background())
	suite.Require().NoError(err)

	codec := suite.service.codecs["vote"]

	native, _, err := codec.NativeFromBinary(msg.Value)
	suite.Require().NoError(err)

	textual, err := codec.TextualFromNative(nil, native)
	suite.Require().NoError(err)

	var voteEvent VoteEvent
	err = json.Unmarshal(textual, &voteEvent)
	suite.Require().NoError(err)

	return voteEvent
}

// This test performs a full e2e test using challenge bypass server to sign use order credentials.
// It uses three tokens and expects three tokens and three signed creds to be returned.
func (suite *ControllersTestSuite) TestE2E_CreateOrderCreds_StoreSignedOrderCredentials_SingleUse() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := os.Getenv("ENV")
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, env)

	// setup kafka
	kafkaUnsignedOrderCredsTopic = os.Getenv("GRANT_CBP_SIGN_CONSUMER_TOPIC")
	kafkaSignedOrderCredsDLQTopic = os.Getenv("GRANT_CBP_SIGN_CONSUMER_TOPIC_DLQ")
	kafkaSignedOrderCredsTopic = os.Getenv("GRANT_CBP_SIGN_PRODUCER_TOPIC")
	kafkaSignedRequestReaderGroupID = test.RandomString()
	ctx = skustest.SetupKafka(ctx, suite.T(), kafkaUnsignedOrderCredsTopic,
		kafkaSignedOrderCredsDLQTopic, kafkaSignedOrderCredsTopic)

	// create macaroon token for sku and whitelist
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{FreeTestSkuToken})

	// create order with order items
	request := model.CreateOrderRequest{
		Email: test.RandomString(),
		Items: []model.OrderItemRequest{
			{
				SKU:      FreeTestSkuToken,
				Quantity: 3,
			},
		},
	}

	client, err := cbr.New()
	suite.Require().NoError(err)

	retryPolicy = retrypolicy.NoRetry // set this so we fail fast

	service := &Service{
		issuerRepo: repository.NewIssuer(),
		Datastore:  suite.storage,
		cbClient:   client,
		retry:      backoff.Retry,
	}

	order, err := service.CreateOrderFromRequest(ctx, request)
	suite.Require().NoError(err)

	// Create order credentials for the newly create order
	data := CreateOrderCredsRequest{
		ItemID: order.Items[0].ID,
		// these are already base64 encoded
		BlindedCreds: []string{"HLLrM7uBm4gVWr8Bsgx3M/yxDHVJX3gNow8Sx6sAPAY=",
			"Hi1j/9Pen5vRvGSLn6eZCxgtkgZX7LU9edmOD2w5CWo=", "YG07TqExOSoo/46SIWK42OG0of3z94Y5SzCswW6sYSw="},
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	requestID := uuid.NewV4().String()
	ctx = context.WithValue(ctx, requestutils.RequestID, requestID)

	r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/%s/credentials",
		order.ID), bytes.NewBuffer(payload)).WithContext(ctx)

	rw := httptest.NewRecorder()

	// Enable store signed order creds consumer
	ctx = context.WithValue(ctx, appctx.SkusEnableStoreSignedOrderCredsConsumer, true)
	ctx = context.WithValue(ctx, appctx.SkusNumberStoreSignedOrderCredsConsumer, 1)

	skuService, err := InitService(ctx, suite.storage, nil, repository.NewOrder(), repository.NewIssuer())
	suite.Require().NoError(err)

	authMwr := NewAuthMwr(skuService)
	instrumentHandler := func(name string, h http.Handler) http.Handler {
		return h
	}

	router := Router(skuService, authMwr, instrumentHandler, newCORSOptsEnv())

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, r)
	suite.Require().Equal(http.StatusOK, rw.Code)

	// Assert the order creds have been submitted
	messages, err := suite.storage.GetSigningOrderRequestOutboxByOrder(ctx, order.ID)
	suite.Require().NoError(err)
	suite.Require().Len(messages, 1)
	suite.Require().Equal(data.ItemID, messages[0].ItemID)

	// start processing out signing request/results from kafka
	go func() {
		for {
			select {
			case <-ctx.Done():
				break
			default:
				skuService.RunStoreSignedOrderCredentials(ctx, 0)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				break
			default:
				_, _ = skuService.RunSendSigningRequestJob(ctx)
				time.Sleep(time.Second)
			}
		}
	}()

	var recorder *httptest.ResponseRecorder
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/%s/credentials", order.ID), nil)
	timeout := time.Now().Add(time.Second * 50)

	for {
		recorder = httptest.NewRecorder()
		server.Handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusAccepted || time.Now().After(timeout) {
			break
		}
		time.Sleep(1 * time.Second)
	}

	suite.Require().Equal(http.StatusOK, recorder.Code)

	var response []OrderCreds
	err = json.NewDecoder(recorder.Body).Decode(&response)
	suite.Require().NoError(err)

	suite.Assert().Equal(order.ID, response[0].OrderID)
	suite.Assert().NotEmpty(response[0].IssuerID)
	suite.Assert().Equal(1, len(response))
	suite.Assert().NotEmpty(response[0].SignedCreds)
}

// This test performs a full e2e test using challenge bypass server to sign time limited v2 order credentials.
// It uses three tokens and expects three signing results (which is determined by the issuer buffer/overlap and CBR)
// which translates to three time limited v2 order credentials being stored for the single order containing
// a single order item.
func (suite *ControllersTestSuite) TestE2E_CreateOrderCreds_StoreSignedOrderCredentials_TimeLimitedV2() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := os.Getenv("ENV")
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, env)

	// setup kafka
	kafkaUnsignedOrderCredsTopic = os.Getenv("GRANT_CBP_SIGN_CONSUMER_TOPIC")
	kafkaSignedOrderCredsDLQTopic = os.Getenv("GRANT_CBP_SIGN_CONSUMER_TOPIC_DLQ")
	kafkaSignedOrderCredsTopic = os.Getenv("GRANT_CBP_SIGN_PRODUCER_TOPIC")
	kafkaSignedRequestReaderGroupID = test.RandomString()
	ctx = skustest.SetupKafka(ctx, suite.T(), kafkaUnsignedOrderCredsTopic,
		kafkaSignedOrderCredsDLQTopic, kafkaSignedOrderCredsTopic)

	// create macaroon token for sku and whitelist
	sku := test.RandomString()
	price := 0
	token := suite.CreateMacaroon(sku, price)
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{token})

	// create order with order items
	request := model.CreateOrderRequest{
		Email: test.RandomString(),
		Items: []model.OrderItemRequest{
			{
				SKU:      token,
				Quantity: 1,
			},
		},
	}

	client, err := cbr.New()
	suite.Require().NoError(err)

	retryPolicy = retrypolicy.NoRetry // set this so we fail fast

	service := &Service{
		issuerRepo: repository.NewIssuer(),
		Datastore:  suite.storage,
		cbClient:   client,
		retry:      backoff.Retry,
	}

	order, err := service.CreateOrderFromRequest(ctx, request)
	suite.Require().NoError(err)

	err = service.Datastore.UpdateOrder(order.ID, OrderStatusPaid) // to update the last paid at
	suite.Require().NoError(err)

	// Create order credentials for the newly create order
	data := CreateOrderCredsRequest{
		ItemID: order.Items[0].ID,
		// these are already base64 encoded
		BlindedCreds: []string{"HLLrM7uBm4gVWr8Bsgx3M/yxDHVJX3gNow8Sx6sAPAY=",
			"Hi1j/9Pen5vRvGSLn6eZCxgtkgZX7LU9edmOD2w5CWo=", "YG07TqExOSoo/46SIWK42OG0of3z94Y5SzCswW6sYSw="},
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	requestID := uuid.NewV4().String()
	ctx = context.WithValue(ctx, requestutils.RequestID, requestID)

	r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/%s/credentials",
		order.ID), bytes.NewBuffer(payload)).WithContext(ctx)

	rw := httptest.NewRecorder()

	// Enable store signed order creds consumer
	ctx = context.WithValue(ctx, appctx.SkusEnableStoreSignedOrderCredsConsumer, true)
	ctx = context.WithValue(ctx, appctx.SkusNumberStoreSignedOrderCredsConsumer, 1)

	skuService, err := InitService(ctx, suite.storage, nil, repository.NewOrder(), repository.NewIssuer())
	suite.Require().NoError(err)

	authMwr := NewAuthMwr(skuService)
	instrumentHandler := func(name string, h http.Handler) http.Handler {
		return h
	}

	router := Router(skuService, authMwr, instrumentHandler, newCORSOptsEnv())

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, r)
	suite.Require().Equal(http.StatusOK, rw.Code)

	// Assert the order creds have been submitted
	messages, err := suite.storage.GetSigningOrderRequestOutboxByOrder(ctx, order.ID)
	suite.Require().NoError(err)
	suite.Require().Len(messages, 1)
	suite.Require().Equal(data.ItemID, messages[0].ItemID)

	// start processing out signing request/results from kafka
	go func() {
		for {
			select {
			case <-ctx.Done():
				break
			default:
				skuService.RunStoreSignedOrderCredentials(ctx, 0)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				break
			default:
				_, _ = skuService.RunSendSigningRequestJob(ctx)
				time.Sleep(time.Second)
			}
		}
	}()

	var recorder *httptest.ResponseRecorder
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/%s/credentials", order.ID), nil)
	timeout := time.Now().Add(time.Second * 50)

	for {
		recorder = httptest.NewRecorder()
		server.Handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusAccepted || time.Now().After(timeout) {
			break
		}
		time.Sleep(1 * time.Second)
	}

	suite.Require().Equal(http.StatusOK, recorder.Code)

	var response []TimeAwareSubIssuedCreds
	err = json.NewDecoder(recorder.Body).Decode(&response)
	suite.Require().NoError(err)

	suite.Assert().Equal(order.ID, response[0].OrderID)
	suite.Assert().NotEmpty(response[0].IssuerID)
	suite.Assert().Equal(3, len(response))
}

func (suite *ControllersTestSuite) TestCreateOrderCreds_SingleUse_ExistingOrderCredentials() {
	ctx := context.Background()
	defer ctx.Done()

	// create macaroon token for sku and whitelist
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{FreeTestSkuToken})

	// create order with order items
	request := model.CreateOrderRequest{
		Email: test.RandomString(),
		Items: []model.OrderItemRequest{
			{
				SKU:      FreeTestSkuToken,
				Quantity: 3,
			},
		},
	}

	client, err := cbr.New()
	suite.Require().NoError(err)

	retryPolicy = retrypolicy.NoRetry // set this so we fail fast

	service := &Service{
		issuerRepo: repository.NewIssuer(),
		Datastore:  suite.storage,
		cbClient:   client,
		retry:      backoff.Retry,
	}

	order, err := service.CreateOrderFromRequest(ctx, request)
	suite.Require().NoError(err)

	// create and send the order credentials request for the newly create order
	data := CreateOrderCredsRequest{
		ItemID: order.Items[0].ID,
		// these are already base64 encoded
		BlindedCreds: []string{"HLLrM7uBm4gVWr8Bsgx3M/yxDHVJX3gNow8Sx6sAPAY=",
			"Hi1j/9Pen5vRvGSLn6eZCxgtkgZX7LU9edmOD2w5CWo=", "YG07TqExOSoo/46SIWK42OG0of3z94Y5SzCswW6sYSw="},
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	requestID := uuid.NewV4().String()
	ctx = context.WithValue(ctx, requestutils.RequestID, requestID)

	r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/%s/credentials",
		order.ID), bytes.NewBuffer(payload)).WithContext(ctx)

	rw := httptest.NewRecorder()

	authMwr := NewAuthMwr(service)
	instrumentHandler := func(name string, h http.Handler) http.Handler {
		return h
	}

	router := Router(service, authMwr, instrumentHandler, newCORSOptsEnv())

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, r)
	suite.Require().Equal(http.StatusOK, rw.Code)

	// Send a second create order request for the same order and order item
	rw = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/%s/credentials",
		order.ID), bytes.NewBuffer(payload)).WithContext(ctx)

	server.Handler.ServeHTTP(rw, r)
	suite.Assert().Equal(http.StatusBadRequest, rw.Code)

	var appError handlers.AppError
	err = json.NewDecoder(rw.Body).Decode(&appError)
	suite.Require().NoError(err)

	suite.Assert().Equal(http.StatusBadRequest, appError.Code)
	suite.Assert().Contains(appError.Error(), ErrCredsAlreadyExist.Error())
}

// ReadSigningOrderRequestMessage reads messages from the unsigned order request topic
func (suite *ControllersTestSuite) ReadSigningOrderRequestMessage(ctx context.Context, topic string) SigningOrderRequest {
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
func (suite *ControllersTestSuite) CreateMacaroon(sku string, price int) string {
	c := macaroon.Caveats{
		"sku":                            sku,
		"price":                          strconv.Itoa(price),
		"description":                    test.RandomString(),
		"currency":                       "usd",
		"credential_type":                "time-limited-v2",
		"credential_valid_duration":      "P1M",
		"each_credential_valid_duration": "P1D",
		"issuer_token_buffer":            strconv.Itoa(3),
		"issuer_token_overlap":           strconv.Itoa(0),
		"allowed_payment_methods":        test.RandomString(),
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

func newCORSOptsEnv() cors.Options {
	origins := strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",")
	dbg, _ := strconv.ParseBool(os.Getenv("DEBUG"))

	return NewCORSOpts(origins, dbg)
}

type mockVendorReceiptValidator struct {
	fnValidateApple  func(ctx context.Context, receipt SubmitReceiptRequestV1) (string, error)
	fnValidateGoogle func(ctx context.Context, receipt SubmitReceiptRequestV1) (string, error)
}

func (v *mockVendorReceiptValidator) validateApple(ctx context.Context, receipt SubmitReceiptRequestV1) (string, error) {
	if v.fnValidateApple == nil {
		return "apple_defaul", nil
	}

	return v.fnValidateApple(ctx, receipt)
}

func (v *mockVendorReceiptValidator) validateGoogle(ctx context.Context, receipt SubmitReceiptRequestV1) (string, error) {
	if v.fnValidateGoogle == nil {
		return "google_default", nil
	}

	return v.fnValidateGoogle(ctx, receipt)
}

type mockGcpRequestValidator struct {
	fnValidate func(ctx context.Context, r *http.Request) error
}

func (m *mockGcpRequestValidator) validate(ctx context.Context, r *http.Request) error {
	if m.fnValidate == nil {
		return nil
	}
	return m.fnValidate(ctx, r)
}

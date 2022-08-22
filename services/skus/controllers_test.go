//go:build integration
// +build integration

package skus

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/services/wallet"
	macarooncmd "github.com/brave-intl/bat-go/tools/macaroon/cmd"
	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/clients/cbr"
	mockcb "github.com/brave-intl/bat-go/libs/clients/cbr/mock"
	"github.com/brave-intl/bat-go/libs/clients/gemini"
	mockgemini "github.com/brave-intl/bat-go/libs/clients/gemini/mock"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	kafkautils "github.com/brave-intl/bat-go/libs/kafka"
	logutils "github.com/brave-intl/bat-go/libs/logging"
	timeutils "github.com/brave-intl/bat-go/libs/time"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	"github.com/go-chi/chi"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	kafka "github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

var (
	// these skus will be generated with the appropriate merchant key in setup
	UserWalletVoteTestSkuToken      string
	UserWalletVoteTestSmallSkuToken string
	AnonCardVoteTestSkuToken        string
	FreeTestSkuToken                string
	FreeTLTestSkuToken              string
	FreeTLTest1MSkuToken            string
	InvalidFreeTestSkuToken         string
)

type ControllersTestSuite struct {
	service  *Service
	mockCB   *mockcb.MockClient
	mockCtrl *gomock.Controller
	suite.Suite
}

func TestControllersTestSuite(t *testing.T) {
	suite.Run(t, new(ControllersTestSuite))
}

func (suite *ControllersTestSuite) SetupSuite() {
	govalidator.SetFieldsRequiredByDefault(true)

	AnonCardC := macarooncmd.Caveats{
		"sku":             "anon-card-vote",
		"description":     "brave anon-card-vote sku token v1",
		"credential_type": "single-use",
		"currency":        "BAT",
		"price":           "0.25",
	}

	UserWalletC := macarooncmd.Caveats{
		"sku":             "user-wallet-vote",
		"description":     "brave user-wallet-vote sku token v1",
		"credential_type": "single-use",
		"currency":        "BAT",
		"price":           "0.25",
	}

	UserWalletSmallC := macarooncmd.Caveats{
		"sku":             "user-wallet-vote",
		"description":     "brave user-wallet-vote sku token v1",
		"credential_type": "single-use",
		"currency":        "BAT",
		"price":           "0.00000000000000001",
	}

	FreeC := macarooncmd.Caveats{
		"sku":             "integration-test-free",
		"description":     "integration test free sku token",
		"credential_type": "single-use",
		"currency":        "BAT",
		"price":           "0.00",
	}

	FreeTLC := macarooncmd.Caveats{
		"sku":                       "integration-test-free",
		"description":               "integration test free sku token",
		"credential_type":           "time-limited",
		"credential_valid_duration": "P1M",
		"currency":                  "BAT",
		"price":                     "0.00",
	}

	FreeTL1MC := macarooncmd.Caveats{
		"sku":                       "integration-test-free-1m",
		"description":               "integration test free sku token",
		"credential_type":           "time-limited",
		"credential_valid_duration": "P1M",
		"issuance_interval":         "P1M",
		"currency":                  "BAT",
		"price":                     "0.00",
	}

	// create sku using key
	UserWalletToken := macarooncmd.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macarooncmd.Caveats{UserWalletC},
	}

	UserWalletSmallToken := macarooncmd.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macarooncmd.Caveats{UserWalletSmallC},
	}

	AnonCardToken := macarooncmd.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macarooncmd.Caveats{AnonCardC},
	}

	FreeTestToken := macarooncmd.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macarooncmd.Caveats{FreeC},
	}

	FreeTLTestToken := macarooncmd.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macarooncmd.Caveats{FreeTLC},
	}

	FreeTLTest1MToken := macarooncmd.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macarooncmd.Caveats{FreeTL1MC},
	}

	var err error
	// setup our global skus
	UserWalletVoteTestSkuToken, err = UserWalletToken.Generate("testing123")
	suite.Require().NoError(err)

	// hacky, put this in development sku check
	skuMap["development"][UserWalletVoteTestSkuToken] = true

	UserWalletVoteTestSmallSkuToken, err = UserWalletSmallToken.Generate("testing123")
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
	pg, err := NewPostgres("", false, "")
	suite.Require().NoError(err, "Failed to get postgres conn")

	m, err := pg.NewMigrate()
	suite.Require().NoError(err, "Failed to create migrate instance")

	ver, dirty, _ := m.Version()
	if dirty {
		suite.Require().NoError(m.Force(int(ver)))
	}
	if ver > 0 {
		suite.Require().NoError(m.Down(), "Failed to migrate down cleanly")
	}

	suite.mockCtrl = gomock.NewController(suite.T())
	suite.mockCB = mockcb.NewMockClient(suite.mockCtrl)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	EncryptionKey = "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0"
	InitEncryptionKeys()

	suite.Require().NoError(pg.Migrate(), "Failed to fully migrate")
	suite.service = &Service{
		Datastore: pg,
		cbClient:  suite.mockCB,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
	}

	suite.CleanDB()
	// encrypt merchant key
	cipher, nonce, err := cryptography.EncryptMessage(byteEncryptionKey, []byte("testing123"))
	suite.Require().NoError(err)

	// create key in db for our brave.com location
	_, err = suite.service.Datastore.CreateKey("brave.com", "brave.com", hex.EncodeToString(cipher), hex.EncodeToString(nonce[:]))
	suite.Require().NoError(err)
}

func (suite *ControllersTestSuite) AfterTest(sn, tn string) {
	suite.CleanDB()
	suite.mockCtrl.Finish()
}

func (suite *ControllersTestSuite) CleanDB() {
	tables := []string{
		"vote_drain", "api_keys", "transactions", "order_creds", "order_cred_issuers", "order_items", "orders"}

	if suite.service != nil {
		for _, table := range tables {
			_, err := suite.service.Datastore.RawDB().Exec("delete from " + table)
			suite.Require().NoError(err, "Failed to get clean table")
		}
	}
}

func (suite *ControllersTestSuite) setupCreateOrder(skuToken string, quantity int) Order {
	handler := CreateOrder(suite.service)

	createRequest := &CreateOrderRequest{
		Items: []OrderItemRequest{
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
	handler.ServeHTTP(rr, req)

	suite.Require().Equal(http.StatusCreated, rr.Code)

	var order Order
	err = json.Unmarshal(rr.Body.Bytes(), &order)
	suite.Require().NoError(err)

	return order
}

func (suite *ControllersTestSuite) TestCreateOrder() {
	order := suite.setupCreateOrder(UserWalletVoteTestSkuToken, 40)

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
	order := suite.setupCreateOrder(FreeTestSkuToken, 10)

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
	handler := CreateOrder(suite.service)

	createRequest := &CreateOrderRequest{
		Items: []OrderItemRequest{
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
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)

	suite.Require().Contains(rr.Body.String(), "Invalid SKU Token provided in request")
}

func (suite *ControllersTestSuite) TestGetOrder() {
	order := suite.setupCreateOrder(UserWalletVoteTestSkuToken, 20)

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
	pg, err := NewPostgres("", false, "")
	suite.Require().NoError(err, "Failed to get postgres conn")

	service := &Service{
		Datastore: pg,
	}
	order := suite.setupCreateOrder(UserWalletVoteTestSkuToken, 1/.25)

	handler := CreateGeminiTransaction(service)

	createRequest := &CreateTransactionRequest{
		ExternalTransactionID: uuid.Must(uuid.FromString("150d7a21-c203-4ba4-8fdf-c5fc36aca004")),
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
	suite.Assert().Equal(createRequest.ExternalTransactionID.String(), transaction.ExternalTransactionID)
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

	order := suite.setupCreateOrder(UserWalletVoteTestSmallSkuToken, 1)

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
		ExternalTransactionID: uuid.Must(uuid.FromString(tInfo.ID)),
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
	suite.Assert().Equal(createRequest.ExternalTransactionID.String(), transaction.ExternalTransactionID)
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
	order := suite.setupCreateOrder(UserWalletVoteTestSkuToken, 12/.25)

	handler := CreateUpholdTransaction(suite.service)

	createRequest := &CreateTransactionRequest{
		ExternalTransactionID: uuid.Must(uuid.FromString("9d5b6a7d-795b-4f02-a91e-25eee2852ebf")),
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
	suite.Assert().Equal(createRequest.ExternalTransactionID.String(), transaction.ExternalTransactionID)
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
	suite.Assert().Equal(createRequest.ExternalTransactionID.String(), transactions[0].ExternalTransactionID)
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

func (suite *ControllersTestSuite) fetchTimeLimitedCredentials(ctx context.Context, service *Service, order Order) (ordercreds []TimeLimitedCreds) {

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

func (suite *ControllersTestSuite) fetchCredentials(ctx context.Context, service *Service, mockCB *mockcb.MockClient, order Order, firstTime bool) (issuerName, issuerPublicKey, sig, preimage string, ordercreds []OrderCreds) {
	issuerName = "brave.com?sku=" + order.Items[0].SKU
	issuerPublicKey = "dHuiBIasUO0khhXsWgygqpVasZhtQraDSZxzJW2FKQ4="
	blindedCred := []string{"XhBPMjh4vMw+yoNjE7C5OtoTz2rCtfuOXO/Vk7UwWzY="}
	blindedCreds := []string{"XhBPMjh4vMw+yoNjE7C5OtoTz2rCtfuOXO/Vk7UwWzY=", "XhBPMjh4vMw+yoNjE7C5OtoTz2rCtfuOXO/Vk7UwWzY="}
	signedCreds := []string{"NJnOyyL6YAKMYo6kSAuvtG+/04zK1VNaD9KdKwuzAjU="}
	proof := "IiKqfk10e7SJ54Ud/8FnCf+sLYQzS4WiVtYAM5+RVgApY6B9x4CVbMEngkDifEBRD6szEqnNlc3KA8wokGV5Cw=="
	sig = "PsavkSWaqsTzZjmoDBmSu6YxQ7NZVrs2G8DQ+LkW5xOejRF6whTiuUJhr9dJ1KlA+79MDbFeex38X5KlnLzvJw=="
	preimage = "125KIuuwtHGEl35cb5q1OLSVepoDTgxfsvwTc7chSYUM2Zr80COP19EuMpRQFju1YISHlnB04XJzZYN2ieT9Ng=="

	credsReq := CreateOrderCredsRequest{
		ItemID:       order.Items[0].ID,
		BlindedCreds: blindedCreds,
	}

	body, err := json.Marshal(&credsReq)
	suite.Require().NoError(err)

	handler := CreateOrderCreds(service)
	req, err := http.NewRequest("POST", "/{orderID}/credentials", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	if firstTime {
		mockCB.EXPECT().CreateIssuer(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(defaultMaxTokensPerIssuer)).Return(nil)
		mockCB.EXPECT().GetIssuer(gomock.Any(), gomock.Eq(issuerName)).Return(&cbr.IssuerResponse{
			Name:      issuerName,
			PublicKey: issuerPublicKey,
		}, nil)
	}
	mockCB.EXPECT().SignCredentials(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(blindedCred)).Return(&cbr.CredentialsIssueResponse{
		BatchProof:   proof,
		SignedTokens: signedCreds,
	}, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)

	// Check to see if we have HTTP Accepted
	handler = GetOrderCreds(service)
	req, err = http.NewRequest("GET", "/{orderID}/credentials", nil)
	suite.Require().NoError(err)

	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// check status code for error, or until status is okay (from accepted)
	for rr.Code != http.StatusOK {
		if rr.Code == http.StatusAccepted {
			select {
			case <-ctx.Done():
				break
			default:
				time.Sleep(500 * time.Millisecond)
				rr = httptest.NewRecorder()
				handler.ServeHTTP(rr, req)
			}
		} else if rr.Code > 299 {
			// error condition bail out
			suite.Require().True(false, "error status code, expecting 2xx")
		}
	}

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

	for rr.Code != http.StatusOK {
		if rr.Code == http.StatusBadRequest {
			break
		}
		select {
		case <-ctx.Done():
			break
		default:
			time.Sleep(50 * time.Millisecond)
			rr = httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}
	}

	suite.Require().Equal(http.StatusOK, rr.Code, "Async signing timed out")

	err = json.Unmarshal([]byte(rr.Body.String()), &ordercreds)
	suite.Require().NoError(err)

	return
}

func (suite *ControllersTestSuite) TestAnonymousCardE2E() {
	numVotes := 1

	// Create connection to Kafka
	// FIXME stick kafka setup in suite setup
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")

	dialer, _, err := kafkautils.TLSDialer()
	suite.Require().NoError(err)
	conn, err := dialer.DialLeader(context.Background(), "tcp", strings.Split(kafkaBrokers, ",")[0], "vote", 0)
	suite.Require().NoError(err)

	// create topics
	err = conn.CreateTopics(kafka.TopicConfig{Topic: voteTopic, NumPartitions: 1, ReplicationFactor: 1})
	suite.Require().NoError(err)

	offset, err := conn.ReadLastOffset()
	suite.Require().NoError(err)

	err = suite.service.InitKafka(context.Background())
	suite.Require().NoError(err, "Failed to initialize kafka")

	// kick off async goroutine to monitor the vote
	// queue of uncommitted votes in postgres, and
	// push the votes through redemption and kafka
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				break
			default:
				_, err := suite.service.RunNextVoteDrainJob(ctx)
				suite.Require().NoError(err, "Failed to drain vote queue")
				_, err = suite.service.RunNextOrderJob(ctx)
				suite.Require().NoError(err, "Failed to drain order queue")
				<-time.After(50 * time.Millisecond)
			}
		}
	}()
	defer cancel()

	// Create the order first
	handler := CreateOrder(suite.service)
	createRequest := &CreateOrderRequest{
		Items: []OrderItemRequest{
			{
				SKU:      AnonCardVoteTestSkuToken,
				Quantity: numVotes,
			},
		},
	}

	body, err := json.Marshal(&createRequest)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/v1/orders", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	req = req.WithContext(context.WithValue(req.Context(), appctx.EnvironmentCTXKey, "development"))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusCreated, rr.Code)

	var order Order
	err = json.Unmarshal([]byte(rr.Body.String()), &order)
	suite.Require().NoError(err)

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
	txn, err := userWallet.PrepareTransaction(altcurrency.BAT, altcurrency.BAT.ToProbi(order.TotalPrice), uphold.SettlementDestination, "bat-go:grant-server.TestAC", "", nil)
	suite.Require().NoError(err)

	walletID, err := uuid.FromString(userWallet.ID)
	suite.Require().NoError(err)

	anonCardRequest := CreateAnonCardTransactionRequest{
		WalletID:    walletID,
		Transaction: txn,
	}

	body, err = json.Marshal(&anonCardRequest)
	suite.Require().NoError(err)

	handler = CreateAnonCardTransaction(suite.service)
	req, err = http.NewRequest("POST", "/{orderID}/transactions/anonymouscard", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusCreated, rr.Code)

	issuerName, issuerPublicKey, sig, preimage, ordercreds := suite.fetchCredentials(ctx, suite.service, suite.mockCB, order, true)

	suite.Require().Equal(len(*(*[]string)(ordercreds[0].SignedCreds)), order.Items[0].Quantity)

	// Test getting the same order by item ID
	handler = GetOrderCredsByID(suite.service)
	req, err = http.NewRequest("GET", "/{orderID}/credentials/{itemID}", nil)
	suite.Require().NoError(err)

	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	rctx.URLParams.Add("itemID", order.Items[0].ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)

	// setup our make vote handler
	handler = MakeVote(suite.service)

	vote := Vote{
		Type:    "auto-contribute",
		Channel: "brave.com",
	}

	voteBytes, err := json.Marshal(&vote)
	suite.Require().NoError(err)
	votePayload := base64.StdEncoding.EncodeToString(voteBytes)

	voteReq := VoteRequest{
		Vote: votePayload,
		Credentials: []CredentialBinding{{
			PublicKey:     issuerPublicKey,
			Signature:     sig,
			TokenPreimage: preimage,
		}},
	}

	body, err = json.Marshal(&voteReq)
	suite.Require().NoError(err)

	// mocked redeem creds
	suite.mockCB.EXPECT().RedeemCredentials(gomock.Any(), gomock.Eq([]cbr.CredentialRedemption{{
		Issuer:        issuerName,
		TokenPreimage: preimage,
		Signature:     sig,
	}}), gomock.Eq(votePayload)).Return(nil)

	// perform post to vote endpoint
	req, err = http.NewRequest("POST", "/vote", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	// actually perform the call
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)

	body, _ = ioutil.ReadAll(rr.Body)

	<-time.After(5 * time.Second)
	// Test the Kafka Event was put into place
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:          strings.Split(kafkaBrokers, ","),
		Topic:            voteTopic,
		Dialer:           suite.service.kafkaDialer,
		MaxWait:          time.Second,
		RebalanceTimeout: time.Second,
		Logger:           kafka.LoggerFunc(log.Printf),
	})

	codec := suite.service.codecs["vote"]

	// :cry:
	err = r.SetOffset(offset)
	suite.Require().NoError(err)

	voteEventBinary, err := r.ReadMessage(context.Background())
	suite.Require().NoError(err)

	voteEvent, _, err := codec.NativeFromBinary(voteEventBinary.Value)
	suite.Require().NoError(err)

	voteEventJSON, err := codec.TextualFromNative(nil, voteEvent)
	suite.Require().NoError(err)

	suite.Assert().Contains(string(voteEventJSON), "id")

	var ve = new(VoteEvent)

	err = json.Unmarshal(voteEventJSON, ve)
	suite.Require().NoError(err)

	suite.Assert().Equal(ve.Type, vote.Type)
	suite.Assert().Equal(ve.Channel, vote.Channel)
	// should be number of credentials for the vote
	suite.Assert().Equal(ve.VoteTally, int64(len(voteReq.Credentials)))
	// check that the funding source matches the issuer
	suite.Assert().Equal(ve.FundingSource, "anonymous-card") // from SKU...
}

func (suite *ControllersTestSuite) TestTimeLimitedCredentialsVerifyPresentation1M() {
	order := suite.setupCreateOrder(FreeTLTest1MSkuToken, 1)

	// setup a key for our merchant
	k := suite.SetupCreateKey(order.MerchantID)
	// GetKey
	keyID, err := uuid.FromString(k.ID)
	suite.Require().NoError(err, "error parsing key id")

	key, err := suite.service.Datastore.GetKey(keyID, false)
	suite.Require().NoError(err, "error getting key from db")
	secret, err := key.GetSecretKey()
	suite.Require().NoError(err, "unable to decrypt secret")

	ordercreds := suite.fetchTimeLimitedCredentials(context.Background(), suite.service, order)

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
	order := suite.setupCreateOrder(FreeTLTestSkuToken, 1)

	// setup a key for our merchant
	k := suite.SetupCreateKey(order.MerchantID)
	// GetKey
	keyID, err := uuid.FromString(k.ID)
	suite.Require().NoError(err, "error parsing key id")

	key, err := suite.service.Datastore.GetKey(keyID, false)
	suite.Require().NoError(err, "error getting key from db")
	secret, err := key.GetSecretKey()
	suite.Require().NoError(err, "unable to decrypt secret")

	ordercreds := suite.fetchTimeLimitedCredentials(context.Background(), suite.service, order)

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

func (suite *ControllersTestSuite) TestResetCredentialsVerifyPresentation() {
	ctx, cancel := context.WithCancel(context.Background())
	var err error
	go func() {
		for {
			select {
			case <-ctx.Done():
				break
			default:
				_, err = suite.service.RunNextOrderJob(ctx)
				suite.Require().NoError(err, "Failed to drain order queue")
				<-time.After(50 * time.Millisecond)
			}
		}
	}()
	defer cancel()

	order := suite.setupCreateOrder(FreeTestSkuToken, 1)

	_, _, _, _, ordercreds := suite.fetchCredentials(ctx, suite.service, suite.mockCB, order, true)
	suite.Require().Equal(len(*(*[]string)(ordercreds[0].SignedCreds)), order.Items[0].Quantity)

	handler := DeleteOrderCreds(suite.service)
	req, err := http.NewRequest("DELETE", "/{orderID}/credentials", nil)
	suite.Require().NoError(err)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	// Need to add faux auth details to context
	req = req.WithContext(context.WithValue(context.WithValue(req.Context(), merchantCtxKey{}, "brave.com"), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	// Reset should succeed
	suite.Require().Equal(http.StatusOK, rr.Code)

	handler = GetOrderCreds(suite.service)
	req, err = http.NewRequest("GET", "/{orderID}/credentials", nil)
	suite.Require().NoError(err)

	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	// Credentials should be cleared out
	suite.Assert().Equal(http.StatusNotFound, rr.Code)

	// Signing after reset should proceed normally
	issuerName, _, sig, preimage, ordercreds := suite.fetchCredentials(ctx, suite.service, suite.mockCB, order, false)
	suite.Require().Equal(len(*(*[]string)(ordercreds[0].SignedCreds)), order.Items[0].Quantity)

	presentation := cbr.CredentialRedemption{
		Issuer:        issuerName,
		TokenPreimage: preimage,
		Signature:     sig,
	}

	presentationBytes, err := json.Marshal(&presentation)
	suite.Require().NoError(err)
	presentationPayload := base64.StdEncoding.EncodeToString(presentationBytes)

	verifyRequest := VerifyCredentialRequestV1{
		Type:         "single-use",
		Version:      1,
		SKU:          "incorrect-sku",
		MerchantID:   "brave.com",
		Presentation: presentationPayload,
	}

	body, err := json.Marshal(&verifyRequest)
	suite.Require().NoError(err)

	handler = VerifyCredentialV1(suite.service)
	req, err = http.NewRequest("POST", "/subscription/verifications", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	// Need to add faux auth details to context
	req = req.WithContext(context.WithValue(req.Context(), merchantCtxKey{}, "brave.com"))

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	// Verification should fail when outer sku does not match inner presentation
	suite.Assert().Equal(http.StatusBadRequest, rr.Code)

	// Correct the SKU
	verifyRequest.SKU = "integration-test-free"

	body, err = json.Marshal(&verifyRequest)
	suite.Require().NoError(err)

	handler = VerifyCredentialV1(suite.service)
	req, err = http.NewRequest("POST", "/subscription/verifications", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	// Need to add faux auth details to context
	req = req.WithContext(context.WithValue(req.Context(), merchantCtxKey{}, "brave.com"))

	// mocked redeem creds
	suite.mockCB.EXPECT().RedeemCredential(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(preimage), gomock.Eq(sig), gomock.Eq(issuerName))

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	// Verification should succeed if SKU and merchant are correct
	suite.Assert().Equal(http.StatusOK, rr.Code)
}

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
	pg, err := NewPostgres("", false, "")
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
	pg, err := NewPostgres("", false, "")
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
			sql.NullString{
				Valid: true, String: "brave.com",
			},
		},
		Status: OrderStatusPaid, LastPaidAt: &lastPaid,
		ExpiresAt: &expiresAt,
		ValidFor:  &valid,
	}

	creds, status, err := suite.service.GetTimeLimitedCreds(ctx, order)
	suite.Require().True(creds == nil, "should not get creds back")
	suite.Require().True(status == http.StatusBadRequest, "should not get creds back")
	suite.Require().Error(err, "should get an error")

}

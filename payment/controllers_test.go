// +build integration

package payment

import (
	"bytes"
	"context"
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
	macarooncmd "github.com/brave-intl/bat-go/cmd/macaroon"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	mockcb "github.com/brave-intl/bat-go/utils/clients/cbr/mock"
	"github.com/brave-intl/bat-go/utils/cryptography"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	kafkautils "github.com/brave-intl/bat-go/utils/kafka"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/go-chi/chi"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	kafka "github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

var (
	// these skus will be generated with the appropriate merchant key in setup
	UserWalletVoteTestSkuToken string
	AnonCardVoteTestSkuToken   string
	FreeTestSkuToken           string
	InvalidFreeTestSkuToken    string
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

	FreeC := macarooncmd.Caveats{
		"sku":             "integration-test-free",
		"description":     "integration test free sku token",
		"credential_type": "single-use",
		"currency":        "BAT",
		"price":           "0.00",
	}

	// create sku using key
	UserWalletToken := macarooncmd.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macarooncmd.Caveats{UserWalletC},
	}

	AnonCardToken := macarooncmd.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macarooncmd.Caveats{AnonCardC},
	}

	FreeTestToken := macarooncmd.Token{
		ID: "id", Version: 2, Location: "brave.com",
		FirstPartyCaveats: []macarooncmd.Caveats{FreeC},
	}

	var err error
	// setup our global skus
	UserWalletVoteTestSkuToken, err = UserWalletToken.Generate("testing123")
	suite.Require().NoError(err)

	AnonCardVoteTestSkuToken, err = AnonCardToken.Generate("testing123")
	suite.Require().NoError(err)

	FreeTestSkuToken, err = FreeTestToken.Generate("testing123")
	suite.Require().NoError(err)

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

func (suite *ControllersTestSuite) E2EOrdersUpholdTransactionsTest() {
	order := suite.setupCreateOrder(UserWalletVoteTestSkuToken, 1/.25)

	handler := CreateUpholdTransaction(suite.service)

	createRequest := &CreateTransactionRequest{
		ExternalTransactionID: uuid.Must(uuid.FromString("150d7a21-c203-4ba4-8fdf-c5fc36aca004")),
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
	suite.Assert().Equal(decimal.NewFromFloat32(1), transaction.Amount)
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

	// Test to make sure we can't submit the same externalTransactionID twice

	req, err = http.NewRequest("POST", "/v1/orders/{orderID}/transactions/uphold", bytes.NewBuffer(body))
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	postReq = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	suite.Require().NoError(err)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, postReq)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)
	suite.Assert().Equal(rr.Body.String(), "{\"message\":\"Error creating the transaction: External Transaction ID: 3db2f74e-df23-42e2-bf25-a302a93baa2d has already been added to the order\",\"code\":400}\n")
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

func generateWallet(t *testing.T) *uphold.Wallet {
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
	err = newWallet.Register("bat-go test card")
	if err != nil {
		t.Fatal(err)
	}
	return newWallet
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
	suite.Assert().Equal(http.StatusAccepted, rr.Code)

	for rr.Code != http.StatusOK {
		if rr.Code == http.StatusAccepted {
			select {
			case <-ctx.Done():
				break
			default:
				time.Sleep(50 * time.Millisecond)
				rr = httptest.NewRecorder()
				handler.ServeHTTP(rr, req)
			}
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

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusCreated, rr.Code)

	var order Order
	err = json.Unmarshal([]byte(rr.Body.String()), &order)
	suite.Require().NoError(err)

	userWallet := generateWallet(suite.T())
	err = suite.service.wallet.Datastore.UpsertWallet(ctx, &userWallet.Info)
	suite.Require().NoError(err)

	balanceBefore, err := userWallet.GetBalance(true)
	balanceAfter, err := uphold.FundWallet(userWallet, order.TotalPrice)
	suite.Require().True(balanceAfter.GreaterThan(balanceBefore.TotalProbi), "balance should have increased")
	txn, err := userWallet.PrepareTransaction(altcurrency.BAT, altcurrency.BAT.ToProbi(order.TotalPrice), uphold.SettlementDestination, "bat-go:grant-server.TestAC")
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
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

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

	verifyRequest := VerifyCredentialRequest{
		Type:         "single-use",
		Version:      1,
		SKU:          "incorrect-sku",
		MerchantID:   "brave.com",
		Presentation: presentationPayload,
	}

	body, err := json.Marshal(&verifyRequest)
	suite.Require().NoError(err)

	handler = VerifyCredential(suite.service)
	req, err = http.NewRequest("POST", "/subscription/verifications", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	// Verification should fail when outer sku does not match inner presentation
	suite.Assert().Equal(http.StatusBadRequest, rr.Code)

	// Correct the SKU
	verifyRequest.SKU = "integration-test-free"

	body, err = json.Marshal(&verifyRequest)
	suite.Require().NoError(err)

	handler = VerifyCredential(suite.service)
	req, err = http.NewRequest("POST", "/subscription/verifications", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	// mocked redeem creds
	suite.mockCB.EXPECT().RedeemCredential(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(preimage), gomock.Eq(sig), gomock.Eq(issuerName))

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	// Verification should succeed if SKU and merchant are correct
	suite.Assert().Equal(http.StatusOK, rr.Code)
}

func (suite *ControllersTestSuite) SetupCreateKey() Key {
	createRequest := &CreateKeyRequest{
		Name: "BAT-GO",
	}

	body, err := json.Marshal(&createRequest)
	suite.Require().NoError(err)
	req, err := http.NewRequest("POST", "/v1/merchants/{merchantID}/key", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	createAPIHandler := CreateKey(suite.service)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("merchantID", "48dc25ed-4121-44ef-8147-4416a76201f7")
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
	Key := suite.SetupCreateKey()

	suite.Assert().Equal("48dc25ed-4121-44ef-8147-4416a76201f7", Key.Merchant)
}

func (suite *ControllersTestSuite) TestDeleteKey() {
	key := suite.SetupCreateKey()

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

	key := suite.SetupCreateKey()

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

	key := suite.SetupCreateKey()
	toDelete := suite.SetupCreateKey()
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

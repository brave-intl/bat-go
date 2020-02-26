// +build integration

package payment

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	mockcb "github.com/brave-intl/bat-go/utils/clients/cbr/mock"
	mockledger "github.com/brave-intl/bat-go/utils/clients/ledger/mock"
	mockreputation "github.com/brave-intl/bat-go/utils/clients/reputation/mock"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	walletservice "github.com/brave-intl/bat-go/wallet/service"
	"github.com/go-chi/chi"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/ed25519"
)

type ControllersTestSuite struct {
	suite.Suite
}

func TestControllersTestSuite(t *testing.T) {
	suite.Run(t, new(ControllersTestSuite))
}

func (suite *ControllersTestSuite) SetupSuite() {
	pg, err := NewPostgres("", false)
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

	suite.Require().NoError(pg.Migrate(), "Failed to fully migrate")
}

func (suite *ControllersTestSuite) setupCreateOrder(quantity int) Order {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	service := &Service{
		datastore: pg,
	}
	handler := CreateOrder(service)

	createRequest := &CreateOrderRequest{
		Items: []OrderItemRequest{
			{
				SKU:      "MDAxN2xvY2F0aW9uIGJyYXZlLmNvbQowMDFhaWRlbnRpZmllciBwdWJsaWMga2V5CjAwMzJjaWQgaWQgPSA1Yzg0NmRhMS04M2NkLTRlMTUtOThkZC04ZTE0N2E1NmI2ZmEKMDAxN2NpZCBjdXJyZW5jeSA9IEJBVAowMDE1Y2lkIHByaWNlID0gMC4yNQowMDJmc2lnbmF0dXJlICRlYyTuJdmlRFuPJ5XFQXjzHFZCLTek0yQ3Yc8JUKC0Cg",
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
	suite.Assert().Equal(http.StatusCreated, rr.Code)

	var order Order
	err = json.Unmarshal(rr.Body.Bytes(), &order)
	suite.Assert().NoError(err)

	return order
}

func (suite *ControllersTestSuite) TestCreateOrder() {
	order := suite.setupCreateOrder(40)

	// Check the order
	suite.Assert().Equal("10", order.TotalPrice.String())
	suite.Assert().Equal("brave.com", order.MerchantID)
	suite.Assert().Equal("pending", order.Status)

	// Check the order items
	suite.Assert().Equal(len(order.Items), 1)
	suite.Assert().Equal("BAT", order.Items[0].Currency)
	suite.Assert().Equal("BAT", order.Currency)
	suite.Assert().Equal("0.25", order.Items[0].Price.String())
	suite.Assert().Equal(40, order.Items[0].Quantity)
	suite.Assert().Equal(decimal.New(10, 0), order.Items[0].Subtotal)
	suite.Assert().Equal(order.ID, order.Items[0].OrderID)
}

func (suite *ControllersTestSuite) TestGetOrder() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	service := &Service{
		datastore: pg,
	}

	order := suite.setupCreateOrder(20)

	req, err := http.NewRequest("GET", "/v1/orders/{orderID}", nil)
	suite.Require().NoError(err)

	getOrderHandler := GetOrder(service)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	getReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	getOrderHandler.ServeHTTP(rr, getReq)
	suite.Assert().Equal(http.StatusOK, rr.Code)

	err = json.Unmarshal(rr.Body.Bytes(), &order)
	suite.Assert().NoError(err)

	suite.Assert().Equal("5", order.TotalPrice.String())
	suite.Assert().Equal("brave.com", order.MerchantID)
	suite.Assert().Equal("pending", order.Status)

	// Check the order items
	suite.Assert().Equal(len(order.Items), 1)
	suite.Assert().Equal("BAT", order.Items[0].Currency)
	suite.Assert().Equal("0.25", order.Items[0].Price.String())
	suite.Assert().Equal(20, order.Items[0].Quantity)
	suite.Assert().Equal(decimal.New(5, 0), order.Items[0].Subtotal)
	suite.Assert().Equal(order.ID, order.Items[0].OrderID)
}

func (suite *ControllersTestSuite) E2EOrdersUpholdTransactionsTest() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	service := &Service{
		datastore: pg,
	}
	order := suite.setupCreateOrder(1 / .25)

	handler := CreateUpholdTransaction(service)

	createRequest := &CreateTransactionRequest{
		ExternalTransactionID: "150d7a21-c203-4ba4-8fdf-c5fc36aca004",
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

	suite.Assert().Equal(http.StatusCreated, rr.Code)

	var transaction Transaction
	err = json.Unmarshal(rr.Body.Bytes(), &transaction)
	suite.Assert().NoError(err)

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
	updatedOrder, err := service.datastore.GetOrder(order.ID)
	suite.Assert().NoError(err)
	suite.Assert().Equal("paid", updatedOrder.Status)

	// Test to make sure we can't submit the same externalTransactionID twice

	req, err = http.NewRequest("POST", "/v1/orders/{orderID}/transactions/uphold", bytes.NewBuffer(body))
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	postReq = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	suite.Require().NoError(err)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, postReq)
	suite.Assert().Equal(http.StatusBadRequest, rr.Code)
	suite.Assert().Equal(rr.Body.String(), "{\"message\":\"Error creating the transaction: External Transaction ID: 3db2f74e-df23-42e2-bf25-a302a93baa2d has already been added to the order\",\"code\":400}\n")
}

func (suite *ControllersTestSuite) TestGetTransactions() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	service := &Service{
		datastore: pg,
	}

	// Delete transactions so we don't run into any validation errors
	_, err = pg.DB.Exec("DELETE FROM transactions;")
	suite.Require().NoError(err)

	// External transaction has 12 BAT
	order := suite.setupCreateOrder(12 / .25)

	handler := CreateUpholdTransaction(service)

	createRequest := &CreateTransactionRequest{
		ExternalTransactionID: "9d5b6a7d-795b-4f02-a91e-25eee2852ebf",
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

	suite.Assert().Equal(http.StatusCreated, rr.Code)

	var transaction Transaction
	err = json.Unmarshal(rr.Body.Bytes(), &transaction)
	suite.Assert().NoError(err)

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
	updatedOrder, err := service.datastore.GetOrder(order.ID)
	suite.Assert().NoError(err)
	suite.Assert().Equal("paid", updatedOrder.Status)

	// Get all the transactions, should only be one

	handler = GetTransactions(service)
	req, err = http.NewRequest("GET", "/v1/orders/{orderID}/transactions", nil)
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	getReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	suite.Require().NoError(err)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, getReq)

	suite.Assert().Equal(http.StatusOK, rr.Code)
	var transactions []Transaction
	err = json.Unmarshal(rr.Body.Bytes(), &transactions)
	suite.Assert().NoError(err)

	// Check the transaction
	suite.Assert().Equal(decimal.NewFromFloat32(12), transactions[0].Amount)
	suite.Assert().Equal("uphold", transactions[0].Kind)
	suite.Assert().Equal("completed", transactions[0].Status)
	suite.Assert().Equal("BAT", transactions[0].Currency)
	suite.Assert().Equal(createRequest.ExternalTransactionID, transactions[0].ExternalTransactionID)
	suite.Assert().Equal(order.ID, transactions[0].OrderID)
}

func fundWallet(t *testing.T, destWallet *uphold.Wallet, amount decimal.Decimal) {
	var donorInfo wallet.Info
	donorInfo.Provider = "uphold"
	donorInfo.ProviderID = os.Getenv("DONOR_WALLET_CARD_ID")
	{
		tmp := altcurrency.BAT
		donorInfo.AltCurrency = &tmp
	}

	donorWalletPublicKeyHex := os.Getenv("DONOR_WALLET_PUBLIC_KEY")
	donorWalletPrivateKeyHex := os.Getenv("DONOR_WALLET_PRIVATE_KEY")
	var donorPublicKey httpsignature.Ed25519PubKey
	var donorPrivateKey ed25519.PrivateKey
	donorPublicKey, err := hex.DecodeString(donorWalletPublicKeyHex)
	if err != nil {
		t.Fatal(err)
	}
	donorPrivateKey, err = hex.DecodeString(donorWalletPrivateKeyHex)
	if err != nil {
		t.Fatal(err)
	}
	donorWallet := &uphold.Wallet{Info: donorInfo, PrivKey: donorPrivateKey, PubKey: donorPublicKey}

	if len(donorWallet.ID) > 0 {
		t.Fatal("FIXME")
	}

	_, err = donorWallet.Transfer(altcurrency.BAT, altcurrency.BAT.ToProbi(amount), destWallet.Info.ProviderID)
	if err != nil {
		t.Fatal(err)
	}

	balance, err := destWallet.GetBalance(true)
	if err != nil {
		t.Error(err)
	}

	if balance.TotalProbi.Equals(decimal.Zero) {
		t.Error("Submit with confirm should result in a balance.")
	}
}

func generateWallet(t *testing.T) *uphold.Wallet {
	var info wallet.Info
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

func (suite *ControllersTestSuite) AnonymousCardTestE2E() {
	suite.T().Skip()

	numVotes := 20

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()
	mockCB := mockcb.NewMockClient(mockCtrl)

	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	service := &Service{
		datastore: pg,
		cbClient:  mockCB,
		wallet: walletservice.Service{
			Datastore: pg,
		},
	}

	// Create the order first
	handler := CreateOrder(service)
	createRequest := &CreateOrderRequest{
		Items: []OrderItemRequest{
			{
				SKU:      "MDAxN2xvY2F0aW9uIGJyYXZlLmNvbQowMDFhaWRlbnRpZmllciBwdWJsaWMga2V5CjAwMzJjaWQgaWQgPSA1Yzg0NmRhMS04M2NkLTRlMTUtOThkZC04ZTE0N2E1NmI2ZmEKMDAxN2NpZCBjdXJyZW5jeSA9IEJBVAowMDE1Y2lkIHByaWNlID0gMC4yNQowMDJmc2lnbmF0dXJlICRlYyTuJdmlRFuPJ5XFQXjzHFZCLTek0yQ3Yc8JUKC0Cg",
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
	suite.Assert().Equal(http.StatusCreated, rr.Code)

	var order Order
	err = json.Unmarshal([]byte(rr.Body.String()), &order)
	suite.Assert().NoError(err)

	userWallet := generateWallet(suite.T())
	err = pg.InsertWallet(&userWallet.Info)
	suite.Assert().NoError(err)

	fundWallet(suite.T(), userWallet, order.TotalPrice)
	txn, err := userWallet.PrepareTransaction(altcurrency.BAT, altcurrency.BAT.ToProbi(order.TotalPrice), uphold.SettlementDestination, "bat-go:grant-server.TestAC")
	suite.Assert().NoError(err)

	walletID, err := uuid.FromString(userWallet.ID)
	suite.Require().NoError(err)

	anonCardRequest := CreateAnonCardTransactionRequest{
		WalletID:    walletID,
		Transaction: txn,
	}

	body, err = json.Marshal(&anonCardRequest)
	suite.Require().NoError(err)

	handler = CreateAnonCardTransaction(service)
	req, err = http.NewRequest("POST", "/{orderID}/transactions/anonymouscard", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Assert().Equal(http.StatusCreated, rr.Code)

	issuerName := "brave.com"
	issuerPublicKey := "dHuiBIasUO0khhXsWgygqpVasZhtQraDSZxzJW2FKQ4="
	blindedCreds := []string{"XhBPMjh4vMw+yoNjE7C5OtoTz2rCtfuOXO/Vk7UwWzY="}
	signedCreds := []string{"NJnOyyL6YAKMYo6kSAuvtG+/04zK1VNaD9KdKwuzAjU="}
	proof := "IiKqfk10e7SJ54Ud/8FnCf+sLYQzS4WiVtYAM5+RVgApY6B9x4CVbMEngkDifEBRD6szEqnNlc3KA8wokGV5Cw=="
	sig := "PsavkSWaqsTzZjmoDBmSu6YxQ7NZVrs2G8DQ+LkW5xOejRF6whTiuUJhr9dJ1KlA+79MDbFeex38X5KlnLzvJw=="
	preimage := "125KIuuwtHGEl35cb5q1OLSVepoDTgxfsvwTc7chSYUM2Zr80COP19EuMpRQFju1YISHlnB04XJzZYN2ieT9Ng=="

	credsReq := CreateOrderCredsRequest{
		ItemID:       order.Items[0].ID,
		BlindedCreds: blindedCreds,
	}

	body, err = json.Marshal(&credsReq)
	suite.Require().NoError(err)

	handler = CreateOrderCreds(service)
	req, err = http.NewRequest("POST", "/{orderID}/credentials", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	mockCB.EXPECT().CreateIssuer(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(defaultMaxTokensPerIssuer)).Return(nil)
	mockCB.EXPECT().GetIssuer(gomock.Any(), gomock.Eq(issuerName)).Return(&cbr.IssuerResponse{
		Name:      issuerName,
		PublicKey: issuerPublicKey,
	}, nil)
	mockCB.EXPECT().SignCredentials(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(blindedCreds)).Return(&cbr.CredentialsIssueResponse{
		BatchProof:   proof,
		SignedTokens: signedCreds,
	}, nil)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Assert().Equal(http.StatusOK, rr.Code)

	handler = GetOrderCreds(service)
	req, err = http.NewRequest("GET", "/{orderID}/credentials", nil)
	suite.Require().NoError(err)

	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	ctx, _ := context.WithTimeout(req.Context(), 500*time.Millisecond)
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

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
	suite.Assert().Equal(http.StatusOK, rr.Code, "Async signing timed out")

	// FIXME read body

	handler = MakeVote(service)

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

	// FIXME re-enable once event is emitted
	//mockCB.EXPECT().RedeemCredentials(gomock.Any(), gomock.Eq([]cbr.CredentialRedemption{{
	//Issuer:        issuerName,
	//TokenPreimage: preimage,
	//Signature:     sig,
	//}}), gomock.Eq(votePayload)).Return(nil)

	body, err = json.Marshal(&voteReq)
	suite.Require().NoError(err)

	req, err = http.NewRequest("POST", "/vote", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	// FIXME check event is emitted
}

func (suite *ControllersTestSuite) TestBraveFundsTransaction() {
	// FIXME stick kafka setup in suite setup
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")

	dialer, err := promotion.TLSDialer()
	suite.Require().NoError(err)
	conn, err := dialer.DialLeader(context.Background(), "tcp", strings.Split(kafkaBrokers, ",")[0], "suggestion", 0)
	suite.Require().NoError(err)

	err = conn.CreateTopics(kafka.TopicConfig{Topic: promotion.SuggestionTopic, NumPartitions: 1, ReplicationFactor: 1})
	suite.Require().NoError(err)

	offset, err := conn.ReadLastOffset()
	suite.Require().NoError(err)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err, "Failed to create wallet keypair")

	walletID := uuid.NewV4()
	bat := altcurrency.BAT
	wallet := wallet.Info{
		ID:          walletID.String(),
		Provider:    "uphold",
		ProviderID:  "-",
		AltCurrency: &bat,
		PublicKey:   hex.EncodeToString(publicKey),
		LastBalance: nil,
	}

	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockReputation.EXPECT().IsWalletReputable(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		true,
		nil,
	)
	mockLedger := mockledger.NewMockClient(mockCtrl)
	mockLedger.EXPECT().GetWallet(gomock.Any(), gomock.Eq(walletID)).Return(&wallet, nil)

	mockCB := mockcb.NewMockClient(mockCtrl)

	promotionService := promotion.InitTestService(mockCB, mockLedger, mockReputation)

	err = promotionService.InitKafka()
	suite.Require().NoError(err, "Failed to initialize kafka")

	promotion, err := promotionService.datastore.CreatePromotion("ugp", 2, decimal.NewFromFloat(0.25), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = promotionService.datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	issuerName := promotion.ID.String() + ":control"
	issuerPublicKey := "dHuiBIasUO0khhXsWgygqpVasZhtQraDSZxzJW2FKQ4="
	blindedCreds := []string{"XhBPMjh4vMw+yoNjE7C5OtoTz2rCtfuOXO/Vk7UwWzY="}
	signedCreds := []string{"NJnOyyL6YAKMYo6kSAuvtG+/04zK1VNaD9KdKwuzAjU="}
	proof := "IiKqfk10e7SJ54Ud/8FnCf+sLYQzS4WiVtYAM5+RVgApY6B9x4CVbMEngkDifEBRD6szEqnNlc3KA8wokGV5Cw=="
	sig := "PsavkSWaqsTzZjmoDBmSu6YxQ7NZVrs2G8DQ+LkW5xOejRF6whTiuUJhr9dJ1KlA+79MDbFeex38X5KlnLzvJw=="
	preimage := "125KIuuwtHGEl35cb5q1OLSVepoDTgxfsvwTc7chSYUM2Zr80COP19EuMpRQFju1YISHlnB04XJzZYN2ieT9Ng=="

	mockCB.EXPECT().CreateIssuer(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(defaultMaxTokensPerIssuer)).Return(nil)
	mockCB.EXPECT().GetIssuer(gomock.Any(), gomock.Eq(issuerName)).Return(&cbr.IssuerResponse{
		Name:      issuerName,
		PublicKey: issuerPublicKey,
	}, nil)
	mockCB.EXPECT().SignCredentials(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(blindedCreds)).Return(&cbr.CredentialsIssueResponse{
		BatchProof:   proof,
		SignedTokens: signedCreds,
	}, nil)

	suite.ClaimGrant(promotionService, wallet, privKey, promotion, blindedCreds)

	handler := MakeSuggestion(promotionService)

	suggestion := promotion.Suggestion{
		Type:    "payment",
		Channel: "brave.com",
	}

	suggestionBytes, err := json.Marshal(&suggestion)
	suite.Require().NoError(err)
	suggestionPayload := base64.StdEncoding.EncodeToString(suggestionBytes)

	suggestionReq := SuggestionRequest{
		Suggestion: suggestionPayload,
		Credentials: []CredentialBinding{{
			PublicKey:     issuerPublicKey,
			Signature:     sig,
			TokenPreimage: preimage,
		}},
	}

	mockCB.EXPECT().RedeemCredentials(gomock.Any(), gomock.Eq([]cbr.CredentialRedemption{{
		Issuer:        issuerName,
		TokenPreimage: preimage,
		Signature:     sig,
	}}), gomock.Eq(suggestionPayload)).Return(nil)

	body, err := json.Marshal(&suggestionReq)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/suggestion", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:          strings.Split(kafkaBrokers, ","),
		Topic:            promotionService.SuggestionTopic,
		Dialer:           promotionService.kafkaDialer,
		MaxWait:          time.Second,
		RebalanceTimeout: time.Second,
		Logger:           kafka.LoggerFunc(log.Printf),
	})
	codec := promotionService.codecs["suggestion"]

	// :cry:
	err = r.SetOffset(offset)
	suite.Require().NoError(err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Assert().Equal(http.StatusOK, rr.Code)

	suggestionEventBinary, err := r.ReadMessage(context.Background())
	suite.Require().NoError(err)

	suggestionEvent, _, err := codec.NativeFromBinary(suggestionEventBinary.Value)
	suite.Require().NoError(err)

	suggestionEventJSON, err := codec.TextualFromNative(nil, suggestionEvent)
	suite.Require().NoError(err)

	eventMap, ok := suggestionEvent.(map[string]interface{})
	suite.Require().True(ok)
	id, ok := eventMap["id"].(string)
	suite.Require().True(ok)
	createdAt, ok := eventMap["createdAt"].(string)
	suite.Require().True(ok)

	suite.Assert().JSONEq(`{
		"id": "`+id+`",
		"createdAt": "`+createdAt+`",
		"type": "`+suggestion.Type+`",
		"channel": "`+suggestion.Channel+`",
		"totalAmount": "0.25",
		"funding": [
			{
				"type": "ugp",
				"amount": "0.25",
				"cohort": "control",
				"promotion": "`+promotion.ID.String()+`"
			}
		]
	}`, string(suggestionEventJSON), "Incorrect suggestion event")
}

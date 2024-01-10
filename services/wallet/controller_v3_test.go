//go:build integration

package wallet_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	mock_reputation "github.com/brave-intl/bat-go/libs/clients/reputation/mock"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/services/wallet"
	"github.com/brave-intl/bat-go/services/wallet/model"
	"github.com/brave-intl/bat-go/services/wallet/storage"
	"github.com/btcsuite/btcutil/base58"
	"github.com/go-chi/chi"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type WalletControllersTestSuite struct {
	suite.Suite
}

func TestWalletControllersTestSuite(t *testing.T) {
	suite.Run(t, new(WalletControllersTestSuite))
}

func (suite *WalletControllersTestSuite) SetupSuite() {
	pg, _, err := wallet.NewPostgres()
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

func (suite *WalletControllersTestSuite) SetupTest() {
	suite.CleanDB()
}

func (suite *WalletControllersTestSuite) TearDownTest() {
	suite.CleanDB()
}

func (suite *WalletControllersTestSuite) CleanDB() {
	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions", "wallet_custodian"}

	pg, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.RawDB().Exec("delete from " + table)
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func (suite *WalletControllersTestSuite) FundWallet(w *uphold.Wallet, probi decimal.Decimal) decimal.Decimal {
	ctx := context.Background()
	balanceBefore, err := w.GetBalance(ctx, true)
	total, err := uphold.FundWallet(ctx, w, probi)
	suite.Require().NoError(err, "an error should not be generated from funding the wallet")
	suite.Require().True(total.GreaterThan(balanceBefore.TotalProbi), "submit with confirm should result in an increased balance")
	return total
}

func (suite *WalletControllersTestSuite) CheckBalance(w *uphold.Wallet, expect decimal.Decimal) {
	balances, err := w.GetBalance(context.Background(), true)
	suite.Require().NoError(err, "an error should not be generated from checking the wallet balance")
	totalProbi := altcurrency.BAT.FromProbi(balances.TotalProbi)
	errMessage := fmt.Sprintf("got an unexpected balance. expected: %s, got %s", expect.String(), totalProbi.String())
	suite.Require().True(expect.Equal(totalProbi), errMessage)
}

func (suite *WalletControllersTestSuite) TestBalanceV3() {
	pg, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres connection")

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	service, _ := wallet.InitService(pg, nil, nil, nil, nil, nil, nil, nil, nil, nil, wallet.DAppConfig{})

	w1 := suite.NewWallet(service, "uphold")

	bat1 := decimal.NewFromFloat(0.000000001)

	suite.FundWallet(w1, bat1)

	// check there is 1 bat in w1
	suite.CheckBalance(w1, bat1)

	// call the balance endpoint and check that you get back a total of 1
	handler := wallet.GetUpholdWalletBalanceV3

	req, err := http.NewRequest("GET", "/v3/wallet/uphold/{paymentID}", nil)
	suite.Require().NoError(err, "wallet claim request could not be created")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("paymentID", w1.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(context.WithValue(req.Context(), appctx.RODatastoreCTXKey, pg))
	req = req.WithContext(context.WithValue(req.Context(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"))

	rr := httptest.NewRecorder()
	handlers.AppHandler(handler).ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code, fmt.Sprintf("status is expected to match %d: %s", http.StatusOK, rr.Body.String()))

	var balance wallet.BalanceResponseV3
	err = json.Unmarshal(rr.Body.Bytes(), &balance)
	suite.Require().NoError(err, "failed to unmarshal balance result")

	suite.Require().Equal(balance.Total, float64(0.000000001), fmt.Sprintf("balance is expected to match %f: %f", balance.Total, float64(1)))

	_, err = pg.RawDB().Exec(`update wallets set provider_id = '' where id = $1`, w1.ID)
	suite.Require().NoError(err, "wallet provider_id could not be set as empty string")

	req, err = http.NewRequest("GET", "/v3/wallet/uphold/{paymentID}", nil)
	suite.Require().NoError(err, "wallet claim request could not be created")

	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("paymentID", w1.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(context.WithValue(req.Context(), appctx.RODatastoreCTXKey, pg))
	req = req.WithContext(context.WithValue(req.Context(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"))

	rr = httptest.NewRecorder()
	handlers.AppHandler(handler).ServeHTTP(rr, req)
	expectingForbidden := fmt.Sprintf("status is expected to match %d: %s", http.StatusForbidden, rr.Body.String())
	suite.Require().Equal(http.StatusForbidden, rr.Code, expectingForbidden)
}

func (suite *WalletControllersTestSuite) TestLinkWalletV3() {
	ctx := context.Background()

	pg, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres connection")

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	service, _ := wallet.InitService(pg, nil, nil, nil, nil, nil, nil, nil, nil, nil, wallet.DAppConfig{})

	w1 := suite.NewWallet(service, "uphold")
	w2 := suite.NewWallet(service, "uphold")
	w3 := suite.NewWallet(service, "uphold")
	w4 := suite.NewWallet(service, "uphold")
	bat1 := decimal.NewFromFloat(0.000000001)
	bat2 := decimal.NewFromFloat(0.000000002)

	suite.FundWallet(w1, bat1)
	suite.FundWallet(w2, bat1)
	suite.FundWallet(w3, bat1)
	suite.FundWallet(w4, bat1)

	anonCard1ID, err := w1.CreateCardAddress(ctx, "anonymous")
	suite.Require().NoError(err, "create anon card must not fail")
	anonCard1UUID := uuid.Must(uuid.FromString(anonCard1ID))

	anonCard2ID, err := w2.CreateCardAddress(ctx, "anonymous")
	suite.Require().NoError(err, "create anon card must not fail")
	anonCard2UUID := uuid.Must(uuid.FromString(anonCard2ID))

	anonCard3ID, err := w3.CreateCardAddress(ctx, "anonymous")
	suite.Require().NoError(err, "create anon card must not fail")
	anonCard3UUID := uuid.Must(uuid.FromString(anonCard3ID))

	w1ProviderID := w1.GetWalletInfo().ProviderID
	w2ProviderID := w2.GetWalletInfo().ProviderID
	w3ProviderID := w3.GetWalletInfo().ProviderID

	zero := decimal.NewFromFloat(0)

	suite.CheckBalance(w1, bat1)
	suite.claimCardV3(service, w1, w3ProviderID, http.StatusOK, bat1, &anonCard3UUID)
	suite.CheckBalance(w1, zero)

	suite.CheckBalance(w2, bat1)
	suite.claimCardV3(service, w2, w1ProviderID, http.StatusOK, zero, &anonCard1UUID)
	suite.CheckBalance(w2, bat1)

	suite.CheckBalance(w2, bat1)
	suite.claimCardV3(service, w2, w1ProviderID, http.StatusOK, bat1, &anonCard3UUID)
	suite.CheckBalance(w2, zero)

	suite.CheckBalance(w3, bat2)
	suite.claimCardV3(service, w3, w2ProviderID, http.StatusOK, bat1, &anonCard3UUID)
	suite.CheckBalance(w3, bat1)

	suite.CheckBalance(w3, bat1)
	suite.claimCardV3(service, w3, w1ProviderID, http.StatusOK, zero, &anonCard2UUID)
	suite.CheckBalance(w3, bat1)
}

func (suite *WalletControllersTestSuite) claimCardV3(
	service *wallet.Service,
	w *uphold.Wallet,
	destination string,
	status int,
	amount decimal.Decimal,
	anonymousAddress *uuid.UUID,
) (*walletutils.Info, string) {
	signedCreationRequest, err := w.PrepareTransaction(*w.AltCurrency, altcurrency.BAT.ToProbi(amount), destination, "", "", nil)

	suite.Require().NoError(err, "transaction must be signed client side")

	// V3 Payload
	reqBody := wallet.LinkUpholdDepositAccountRequest{
		SignedLinkingRequest: signedCreationRequest,
	}

	if anonymousAddress != nil {
		reqBody.AnonymousAddress = anonymousAddress.String()
	}

	body, err := json.Marshal(&reqBody)
	suite.Require().NoError(err, "unable to marshal claim body")

	info := w.GetWalletInfo()

	// V3 Handler

	handler := wallet.LinkUpholdDepositAccountV3(service)

	req, err := http.NewRequest("POST", "/v3/wallet/{paymentID}/claim", bytes.NewBuffer(body))
	suite.Require().NoError(err, "wallet claim request could not be created")

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	repClient := mock_reputation.NewMockClient(ctrl)
	repClient.EXPECT().IsLinkingReputable(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("paymentID", info.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(context.WithValue(req.Context(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"))
	req = req.WithContext(context.WithValue(req.Context(), appctx.ReputationClientCTXKey, repClient))

	rr := httptest.NewRecorder()
	handlers.AppHandler(handler).ServeHTTP(rr, req)
	suite.Require().Equal(status, rr.Code, fmt.Sprintf("status is expected to match %d: %s", status, rr.Body.String()))
	linked, err := service.Datastore.GetWallet(req.Context(), uuid.Must(uuid.FromString(w.ID)))
	suite.Require().NoError(err, "retrieving the wallet did not cause an error")
	return linked, rr.Body.String()
}

func (suite *WalletControllersTestSuite) createBody(tx string) string {
	reqBody, _ := json.Marshal(wallet.UpholdCreationRequest{
		SignedCreationRequest: tx,
	})
	return string(reqBody)
}

func (suite *WalletControllersTestSuite) NewWallet(service *wallet.Service, provider string) *uphold.Wallet {
	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	publicKeyString := hex.EncodeToString(publicKey)

	bat := altcurrency.BAT
	info := walletutils.Info{
		ID:          uuid.NewV4().String(),
		PublicKey:   publicKeyString,
		Provider:    provider,
		AltCurrency: &bat,
	}
	w := &uphold.Wallet{
		Info:    info,
		PrivKey: privKey,
		PubKey:  publicKey,
	}

	reg, err := w.PrepareRegistration("Brave Browser Test Link")
	suite.Require().NoError(err, "unable to prepare transaction")

	createResp := suite.createUpholdWalletV3(
		service,
		suite.createBody(reg),
		http.StatusCreated,
		publicKey,
		privKey,
		true,
	)

	var returnedInfo wallet.ResponseV3
	err = json.Unmarshal([]byte(createResp), &returnedInfo)
	suite.Require().NoError(err, "unable to create wallet")
	convertedInfo := wallet.ResponseV3ToInfo(returnedInfo)
	w.Info = *convertedInfo
	return w
}

func (suite *WalletControllersTestSuite) TestCreateBraveWalletV3() {
	pg, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres connection")

	service, _ := wallet.InitService(pg, nil, nil, nil, nil, nil, nil, nil, nil, nil, wallet.DAppConfig{})

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)

	// assume 400 is already covered
	// fail because of lacking signature presence
	notSignedResponse := suite.createUpholdWalletV3(
		service,
		`{}`,
		http.StatusBadRequest,
		publicKey,
		privKey,
		false,
	)

	suite.Assert().JSONEq(`{"code":400, "data":{"validationErrors":{"decoding":"failed decoding: failed to decode signed creation request: unexpected end of JSON input", "signedCreationRequest":"value is required", "validation":"failed validation: missing signed creation request"}}, "message":"Error validating uphold create wallet request validation errors"}`, notSignedResponse, "field is not valid")

	createResp := suite.createBraveWalletV3(
		service,
		``,
		http.StatusCreated,
		publicKey,
		privKey,
		true,
	)

	var created wallet.ResponseV3
	err = json.Unmarshal([]byte(createResp), &created)
	suite.Require().NoError(err, "unable to unmarshal response")

	getResp := suite.getWallet(service, uuid.Must(uuid.FromString(created.PaymentID)), http.StatusOK)

	var gotten wallet.ResponseV3
	err = json.Unmarshal([]byte(getResp), &gotten)
	suite.Require().NoError(err, "unable to unmarshal response")
	// does not return wallet provider
	suite.Require().Equal(created, gotten, "the get and create return the same structure")
}

func (suite *WalletControllersTestSuite) TestCreateUpholdWalletV3() {
	pg, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres connection")

	service, _ := wallet.InitService(pg, nil, nil, nil, nil, nil, nil, nil, nil, nil, wallet.DAppConfig{})

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)

	badJSONBodyParse := suite.createUpholdWalletV3(
		service,
		``,
		http.StatusBadRequest,
		publicKey,
		privKey,
		true,
	)
	suite.Assert().JSONEq(`{
	"code":400,
	"data": {
		"validationErrors":{
			"decoding":"failed decoding: failed to decode json: EOF",
			"signedCreationRequest":"value is required",
			"validation":"failed validation: missing signed creation request"
		}
	},
	"message":"Error validating uphold create wallet request validation errors"
	}`, badJSONBodyParse, "should fail when parsing json")

	badFieldResponse := suite.createUpholdWalletV3(
		service,
		`{"signedCreationRequest":""}`,
		http.StatusBadRequest,
		publicKey,
		privKey,
		true,
	)

	suite.Assert().JSONEq(`{
	"code":400, "data":{"validationErrors":{"decoding":"failed decoding: failed to decode signed creation request: unexpected end of JSON input", "signedCreationRequest":"value is required", "validation":"failed validation: missing signed creation request"}}, "message":"Error validating uphold create wallet request validation errors"}`, badFieldResponse, "field is not valid")

	// assume 403 is already covered
	// fail because of lacking signature presence
	notSignedResponse := suite.createUpholdWalletV3(
		service,
		`{}`,
		http.StatusBadRequest,
		publicKey,
		privKey,
		false,
	)

	suite.Assert().JSONEq(`{
"code":400, "data":{"validationErrors":{"decoding":"failed decoding: failed to decode signed creation request: unexpected end of JSON input", "signedCreationRequest":"value is required", "validation":"failed validation: missing signed creation request"}}, "message":"Error validating uphold create wallet request validation errors"
	}`, notSignedResponse, "field is not valid")
}

func (suite *WalletControllersTestSuite) TestChallenges_Success() {
	paymentID := uuid.NewV4()

	body := struct {
		PaymentID uuid.UUID `json:"paymentId"`
	}{
		PaymentID: paymentID,
	}

	b, err := json.Marshal(body)
	suite.Require().NoError(err)

	r := httptest.NewRequest(http.MethodPost, "/v3/wallet/challenges", bytes.NewBuffer(b))
	r.Header.Set("origin", "https://my-dapp.com")

	rw := httptest.NewRecorder()

	pg, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	chlRep := storage.NewChallenge()

	dac := wallet.DAppConfig{
		AllowedOrigin: "https://my-dapp.com",
	}

	s, err := wallet.InitService(pg, nil, chlRep, nil, nil, nil, nil, nil, nil, nil, dac)
	suite.Require().NoError(err)

	svr := &http.Server{Addr: ":8080", Handler: setupRouter(s)}
	svr.Handler.ServeHTTP(rw, r)
	suite.Require().Equal(http.StatusCreated, rw.Code)
	suite.Require().Equal("https://my-dapp.com", rw.Header().Get("Access-Control-Allow-Origin"))

	type chlResp struct {
		Nonce string `json:"challengeId"`
	}

	var resp chlResp
	err = json.Unmarshal(rw.Body.Bytes(), &resp)
	suite.Require().NoError(err)

	chlRepo := storage.NewChallenge()
	chl, err := chlRepo.Get(context.TODO(), pg.RawDB(), paymentID)
	suite.Require().NoError(err)

	suite.Assert().Equal(chl.Nonce, resp.Nonce)
}

func (suite *WalletControllersTestSuite) TestLinkSolanaAddress_Success() {
	viper.Set("enable-link-drain-flag", "true")

	pg, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	chlRep := storage.NewChallenge()
	allowList := storage.NewAllowList()

	// create the wallet
	pub, priv, err := ed25519.GenerateKey(nil)
	suite.Require().NoError(err)

	paymentID := uuid.NewV4()
	w := &walletutils.Info{
		ID:          paymentID.String(),
		Provider:    "brave",
		PublicKey:   hex.EncodeToString(pub),
		AltCurrency: ptrTo(altcurrency.BAT),
	}
	err = pg.InsertWallet(context.TODO(), w)
	suite.Require().NoError(err)

	whitelistWallet(suite.T(), pg, w.ID)

	// create nonce
	chl := model.NewChallenge(paymentID)

	err = chlRep.Upsert(context.TODO(), pg.RawDB(), chl)
	suite.Require().NoError(err)

	dac := wallet.DAppConfig{
		AllowedOrigin: "https://my-dapp.com",
	}

	s, err := wallet.InitService(pg, nil, chlRep, allowList, nil, nil, nil, nil, nil, nil, dac)
	suite.Require().NoError(err)

	// create linking message
	solPub, msg, solSig := createAndSignMessage(suite.T(), w.ID, priv, chl.Nonce)

	// make request
	body := struct {
		SolanaPublicKey string `json:"solanaPublicKey"`
		Message         string `json:"message"`
		SolanaSignature string `json:"solanaSignature"`
	}{
		SolanaPublicKey: solPub,
		Message:         msg,
		SolanaSignature: solSig,
	}

	b, err := json.Marshal(body)
	suite.Require().NoError(err)

	r := httptest.NewRequest(http.MethodPost, "/v3/wallet/solana/"+w.ID+"/connect", bytes.NewBuffer(b))
	r.Header.Set("origin", "https://my-dapp.com")

	rw := httptest.NewRecorder()

	svr := &http.Server{Addr: ":8080", Handler: setupRouter(s)}
	svr.Handler.ServeHTTP(rw, r)
	suite.Require().Equal(http.StatusOK, rw.Code)
	suite.Require().Equal("https://my-dapp.com", rw.Header().Get("Access-Control-Allow-Origin"))

	// assert
	actual, err := pg.GetWallet(context.TODO(), paymentID)
	suite.Require().NoError(err)

	suite.Require().Equal(solPub, actual.UserDepositDestination)

	// after a successful linking the challenge should be removed from the database.
	_, actualErr := chlRep.Get(context.TODO(), pg.RawDB(), paymentID)
	suite.Assert().ErrorIs(actualErr, model.ErrNotFound)
}

func whitelistWallet(t *testing.T, pg wallet.Datastore, Id string) {
	const q = `insert into allow_list (payment_id, created_at) values($1, $2)`
	_, err := pg.RawDB().Exec(q, Id, time.Now())
	require.NoError(t, err)
}

func createAndSignMessage(t *testing.T, paymentID string, rewardsPrivKey ed25519.PrivateKey, nonce string) (solPub, msg, solSig string) {
	// Create and sign the rewards message.
	// The message has the format <rewardsMessage> = <rewards-payment-id>.<nonce>
	rewardsMsg := paymentID + "." + nonce
	sig, err := rewardsPrivKey.Sign(rand.Reader, []byte(rewardsMsg), crypto.Hash(0))
	require.NoError(t, err)

	rewardsSig := base64.URLEncoding.EncodeToString(sig)
	rewardsPart := rewardsMsg + "." + rewardsSig

	// Create the linking message and sign with the Solana key.
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	solPub = base58.Encode(pub)

	const msgTmpl = "<some-text>:%s\n<some-text>:%s\n<some-text>:%s"
	msg = fmt.Sprintf(msgTmpl, paymentID, solPub, rewardsPart)

	sig, err = priv.Sign(rand.Reader, []byte(msg), crypto.Hash(0))
	require.NoError(t, err)

	solSig = base64.URLEncoding.EncodeToString(sig)

	return
}

func (suite *WalletControllersTestSuite) getWallet(service *wallet.Service, paymentId uuid.UUID, code int) string {
	handler := handlers.AppHandler(wallet.GetWalletV3)

	req, err := http.NewRequest("GET", "/v3/wallet/"+paymentId.String(), nil)
	suite.Require().NoError(err, "a request should be created")

	req = req.WithContext(context.WithValue(req.Context(), appctx.DatastoreCTXKey, service.Datastore))
	req = req.WithContext(context.WithValue(req.Context(), appctx.RODatastoreCTXKey, service.Datastore))
	req = req.WithContext(context.WithValue(req.Context(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("paymentID", paymentId.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Require().Equal(code, rr.Code, "known status code should be sent: "+rr.Body.String())

	return rr.Body.String()
}

func (suite *WalletControllersTestSuite) TestGetWalletV4() {
	pg, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	pub, _, err := ed25519.GenerateKey(nil)
	suite.Require().NoError(err)

	paymentID := uuid.NewV4()
	w := &walletutils.Info{
		ID:          paymentID.String(),
		Provider:    "brave",
		PublicKey:   hex.EncodeToString(pub),
		AltCurrency: ptrTo(altcurrency.BAT),
	}
	err = pg.InsertWallet(context.TODO(), w)
	suite.Require().NoError(err)

	whitelistWallet(suite.T(), pg, w.ID)

	allowList := storage.NewAllowList()

	service, _ := wallet.InitService(pg, nil, nil, allowList, nil, nil, nil, nil, nil, nil, wallet.DAppConfig{})

	handler := handlers.AppHandler(wallet.GetWalletV4(service))

	req, err := http.NewRequest("GET", "/v4/wallets/"+paymentID.String(), nil)
	suite.Require().NoError(err, "a request should be created")

	req = req.WithContext(context.WithValue(req.Context(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("paymentID", paymentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Assert().Equal(http.StatusOK, rr.Code)

	var resp wallet.ResponseV4
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	suite.Require().NoError(err)

	suite.Assert().Equal(true, resp.SelfCustodyAvailable["solana"])
}

func (suite *WalletControllersTestSuite) createBraveWalletV3(
	service *wallet.Service,
	body string,
	code int,
	publicKey httpsignature.Ed25519PubKey,
	privateKey ed25519.PrivateKey,
	shouldSign bool,
) string {

	handler := handlers.AppHandler(wallet.CreateBraveWalletV3)

	bodyBuffer := bytes.NewBuffer([]byte(body))
	req, err := http.NewRequest("POST", "/v3/wallet/brave", bodyBuffer)
	suite.Require().NoError(err, "a request should be created")

	// setup context
	req = req.WithContext(context.WithValue(context.Background(), appctx.DatastoreCTXKey, service.Datastore))
	req = req.WithContext(context.WithValue(req.Context(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"))

	if shouldSign {
		suite.SignRequest(
			req,
			publicKey,
			privateKey,
		)
	}

	rctx := chi.NewRouteContext()
	joined := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	req = req.WithContext(joined)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(code, rr.Code, "known status code should be sent: "+rr.Body.String())

	return rr.Body.String()
}

func (suite *WalletControllersTestSuite) createUpholdWalletV3(
	service *wallet.Service,
	body string,
	code int,
	publicKey httpsignature.Ed25519PubKey,
	privateKey ed25519.PrivateKey,
	shouldSign bool,
) string {

	handler := handlers.AppHandler(wallet.CreateUpholdWalletV3)

	bodyBuffer := bytes.NewBuffer([]byte(body))
	req, err := http.NewRequest("POST", "/v3/wallet", bodyBuffer)
	suite.Require().NoError(err, "a request should be created")

	// setup context
	req = req.WithContext(context.WithValue(context.Background(), appctx.DatastoreCTXKey, service.Datastore))
	req = req.WithContext(context.WithValue(req.Context(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"))

	if shouldSign {
		suite.SignRequest(
			req,
			publicKey,
			privateKey,
		)
	}

	rctx := chi.NewRouteContext()
	joined := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	req = req.WithContext(joined)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(code, rr.Code, "known status code should be sent: "+rr.Body.String())

	return rr.Body.String()
}

func (suite *WalletControllersTestSuite) SignRequest(req *http.Request, publicKey httpsignature.Ed25519PubKey, privateKey ed25519.PrivateKey) {
	var s httpsignature.SignatureParams
	s.Algorithm = httpsignature.ED25519
	s.KeyID = hex.EncodeToString(publicKey)
	s.Headers = []string{"digest", "(request-target)"}

	err := s.Sign(privateKey, crypto.Hash(0), req)
	suite.Require().NoError(err)
}

func setupRouter(service *wallet.Service) *chi.Mux {
	mw := func(name string, h http.Handler) http.Handler {
		return h
	}
	s := "https://my-dapp.com"
	r := chi.NewRouter()
	r.Mount("/v3", wallet.RegisterRoutes(context.TODO(), service, r, mw, wallet.NewDAppCorsMw(s)))
	return r
}

func ptrTo[T any](v T) *T {
	return &v
}

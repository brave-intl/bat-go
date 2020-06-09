// +build integration

package wallet

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	uphold "github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type WalletControllersTestSuite struct {
	suite.Suite
}

func TestWalletControllersTestSuite(t *testing.T) {
	suite.Run(t, new(WalletControllersTestSuite))
}

func (suite *WalletControllersTestSuite) SetupSuite() {
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

func (suite *WalletControllersTestSuite) SetupTest() {
	suite.CleanDB()
}

func (suite *WalletControllersTestSuite) TearDownTest() {
	suite.CleanDB()
}

func (suite *WalletControllersTestSuite) CleanDB() {
	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions"}

	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.DB.Exec("delete from " + table)
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func noUUID() *uuid.UUID {
	return nil
}

func (suite *WalletControllersTestSuite) FundWallet(w *uphold.Wallet, probi decimal.Decimal) decimal.Decimal {
	balanceBefore, err := w.GetBalance(true)
	total, err := uphold.FundWallet(w, probi)
	suite.Require().NoError(err, "an error should not be generated from funding the wallet")
	suite.Require().True(total.GreaterThan(balanceBefore.TotalProbi), "submit with confirm should result in an increased balance")
	return total
}

func (suite *WalletControllersTestSuite) CheckBalance(w *uphold.Wallet, expect decimal.Decimal) {
	balances, err := w.GetBalance(true)
	suite.Require().NoError(err, "an error should not be generated from checking the wallet balance")
	totalProbi := altcurrency.BAT.FromProbi(balances.TotalProbi)
	errMessage := fmt.Sprintf("got an unexpected balance. expected: %s, got %s", expect.String(), totalProbi.String())
	suite.Require().True(expect.Equal(totalProbi), errMessage)
}

func (suite *WalletControllersTestSuite) TestLinkWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres connection")

	service := &Service{
		Datastore: pg,
	}

	w1 := suite.NewWallet(service, "uphold")
	w2 := suite.NewWallet(service, "uphold")
	w3 := suite.NewWallet(service, "uphold")
	w4 := suite.NewWallet(service, "uphold")
	bat1 := decimal.NewFromFloat(1)

	suite.FundWallet(w1, bat1)
	suite.FundWallet(w2, bat1)
	suite.FundWallet(w3, bat1)
	suite.FundWallet(w4, bat1)
	settlement := os.Getenv("BAT_SETTLEMENT_ADDRESS")

	anonCard1ID, err := w1.CreateCardAddress("anonymous")
	suite.Require().NoError(err, "create anon card must not fail")
	anonCard1UUID := uuid.Must(uuid.FromString(anonCard1ID))

	anonCard2ID, err := w2.CreateCardAddress("anonymous")
	suite.Require().NoError(err, "create anon card must not fail")
	anonCard2UUID := uuid.Must(uuid.FromString(anonCard2ID))

	w1ProviderID := w1.GetWalletInfo().ProviderID
	w2ProviderID := w2.GetWalletInfo().ProviderID
	w3ProviderID := w3.GetWalletInfo().ProviderID

	zero := decimal.NewFromFloat(0)

	suite.CheckBalance(w1, bat1)
	suite.claimCard(service, w1, settlement, http.StatusOK, bat1, noUUID())
	suite.CheckBalance(w1, zero)

	suite.CheckBalance(w2, bat1)
	suite.claimCard(service, w2, w1ProviderID, http.StatusOK, zero, &anonCard1UUID)
	suite.CheckBalance(w2, bat1)

	suite.CheckBalance(w2, bat1)
	suite.claimCard(service, w2, w1ProviderID, http.StatusOK, bat1, noUUID())
	suite.CheckBalance(w2, zero)

	suite.CheckBalance(w3, bat1)
	suite.claimCard(service, w3, w2ProviderID, http.StatusOK, bat1, noUUID())
	suite.CheckBalance(w3, zero)

	suite.CheckBalance(w4, bat1)
	suite.claimCard(service, w4, w3ProviderID, http.StatusConflict, bat1, noUUID())
	suite.CheckBalance(w4, bat1)

	suite.CheckBalance(w3, zero)
	suite.claimCard(service, w3, settlement, http.StatusOK, zero, &anonCard2UUID)
	suite.CheckBalance(w3, zero)
}

func (suite *WalletControllersTestSuite) claimCard(
	service *Service,
	w *uphold.Wallet,
	destination string,
	status int,
	amount decimal.Decimal,
	anonymousAddress *uuid.UUID,
) (*walletutils.Info, string) {
	signedTx, err := w.PrepareTransaction(*w.AltCurrency, altcurrency.BAT.ToProbi(amount), destination, "")
	suite.Require().NoError(err, "transaction must be signed client side")
	reqBody := LinkWalletRequest{
		SignedTx:         signedTx,
		AnonymousAddress: anonymousAddress,
	}
	body, err := json.Marshal(&reqBody)
	suite.Require().NoError(err, "unable to marshal claim body")
	handler := PostLinkWalletCompat(service)
	req, err := http.NewRequest("POST", "/v1/wallet/{paymentID}/claim", bytes.NewBuffer(body))
	suite.Require().NoError(err, "wallet claim request could not be created")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("paymentID", w.GetWalletInfo().ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(status, rr.Code, fmt.Sprintf("status is expected to match %d: %s", status, rr.Body.String()))
	linked, err := service.Datastore.GetWallet(uuid.Must(uuid.FromString(w.ID)))
	suite.Require().NoError(err, "retrieving the wallet did not cause an error")
	return linked, rr.Body.String()
}

func (suite *WalletControllersTestSuite) createBody(
	provider string,
	publicKey string,
	tx string,
) string {
	return `{"provider":"` + provider + `","signedTx":"` + tx + `"}`
}

func (suite *WalletControllersTestSuite) NewWallet(service *Service, provider string) *uphold.Wallet {
	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	publicKeyString := hex.EncodeToString(publicKey)

	bat := altcurrency.BAT
	info := walletutils.Info{
		ID:          uuid.NewV4().String(),
		PublicKey:   publicKeyString,
		Provider:    "uphold",
		AltCurrency: &bat,
	}
	wallet := &uphold.Wallet{
		Info:    info,
		PrivKey: privKey,
		PubKey:  publicKey,
	}

	reg, err := wallet.PrepareRegistration("Brave Browser Test Link")
	suite.Require().NoError(err, "unable to prepare transaction")

	createResp := suite.createWallet(
		service,
		suite.createBody(provider, publicKeyString, reg),
		http.StatusCreated,
		publicKey,
		privKey,
		true,
	)

	var returnedInfo walletutils.Info
	err = json.Unmarshal([]byte(createResp), &returnedInfo)
	suite.Require().NoError(err, "unable to create wallet")
	wallet.Info = returnedInfo
	return wallet
}

func (suite *WalletControllersTestSuite) TestPostCreateWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres connection")
	service := &Service{
		Datastore: pg,
	}

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	createBody := func(provider string) string {
		return `{"provider":"` + provider + `"}`
	}
	badJSONBodyParse := suite.createWallet(
		service,
		``,
		http.StatusBadRequest,
		publicKey,
		privKey,
		true,
	)
	suite.Assert().JSONEq(`{
		"message": "Error unmarshalling body: error unmarshalling body",
		"code": 400
	}`, badJSONBodyParse, "should fail when parsing json")

	badFieldResponse := suite.createWallet(
		service,
		createBody("notaprovider"),
		http.StatusBadRequest,
		publicKey,
		privKey,
		true,
	)
	suite.Assert().JSONEq(`{
		"code": 400,
		"message": "Error validating request body",
		"data": {
			"validationErrors": {
				"provider": "notaprovider does not validate as in(brave|uphold)"
			}
		}
	}`, badFieldResponse, "field is not valid")

	// assume 403 is already covered
	// fail because of lacking signature presence
	notSignedResponse := suite.createWallet(
		service,
		createBody("brave"),
		http.StatusBadRequest,
		publicKey,
		privKey,
		false,
	)
	suite.Assert().Equal(`Bad Request
`, notSignedResponse, "not signed creation requests should fail")
	createResp := suite.createWallet(
		service,
		createBody("brave"),
		http.StatusCreated,
		publicKey,
		privKey,
		true,
	)

	var created walletutils.Info
	err = json.Unmarshal([]byte(createResp), &created)
	suite.Require().NoError(err, "unable to unmarshal response")

	getResp := suite.getWallet(service, uuid.Must(uuid.FromString(created.ID)), http.StatusOK)

	var gotten walletutils.Info
	err = json.Unmarshal([]byte(getResp), &gotten)
	suite.Require().NoError(err, "unable to unmarshal response")
	// gotten.PrivateKey = created.PrivateKey
	suite.Require().Equal(created, gotten, "the get and create return the same structure")
}

func (suite *WalletControllersTestSuite) getWallet(
	service *Service,
	paymentId uuid.UUID,
	code int,
) string {
	handler := GetWallet(service)

	req, err := http.NewRequest("GET", "/v1/wallet/"+paymentId.String(), nil)
	suite.Require().NoError(err, "a request should be created")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("paymentId", paymentId.String())
	joined := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	req = req.WithContext(joined)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Require().Equal(code, rr.Code, "known status code should be sent")

	return rr.Body.String()
}

func (suite *WalletControllersTestSuite) createWallet(
	service *Service,
	body string,
	code int,
	publicKey httpsignature.Ed25519PubKey,
	privateKey ed25519.PrivateKey,
	shouldSign bool,
) string {
	handler := middleware.HTTPSignedOnly(service)(PostCreateWallet(service))

	bodyBuffer := bytes.NewBuffer([]byte(body))
	req, err := http.NewRequest("POST", "/v1/wallet", bodyBuffer)
	suite.Require().NoError(err, "a request should be created")

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

func (suite *WalletControllersTestSuite) SignRequest(
	req *http.Request,
	publicKey httpsignature.Ed25519PubKey,
	privateKey ed25519.PrivateKey,
) {
	var s httpsignature.Signature
	s.Algorithm = httpsignature.ED25519
	s.KeyID = hex.EncodeToString(publicKey)
	s.Headers = []string{"digest", "(request-target)"}

	err := s.Sign(privateKey, crypto.Hash(0), req)
	suite.Require().NoError(err)
}

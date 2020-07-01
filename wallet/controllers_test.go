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

	"github.com/brave-intl/bat-go/utils/altcurrency"
	mockledger "github.com/brave-intl/bat-go/utils/clients/ledger/mock"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	uphold "github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/go-chi/chi"
	gomock "github.com/golang/mock/gomock"
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
	pg, _, err := NewPostgres()
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

	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.RawDB().Exec("delete from " + table)
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

func (suite *WalletControllersTestSuite) TestLinkWalletV3() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres connection")

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	mockLedger := mockledger.NewMockClient(mockCtrl)

	service := &Service{
		Datastore:    pg,
		LedgerClient: mockLedger,
	}

	w1 := suite.NewWallet(service, "uphold")
	w2 := suite.NewWallet(service, "uphold")
	w3 := suite.NewWallet(service, "uphold")
	w4 := suite.NewWallet(service, "uphold")
	bat1 := decimal.NewFromFloat(1)

	fmt.Printf("%#v\n", w1)
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
	suite.claimCardV3(service, mockLedger, w1, settlement, http.StatusOK, bat1, noUUID())
	suite.CheckBalance(w1, zero)

	suite.CheckBalance(w2, bat1)
	suite.claimCardV3(service, mockLedger, w2, w1ProviderID, http.StatusOK, zero, &anonCard1UUID)
	suite.CheckBalance(w2, bat1)

	suite.CheckBalance(w2, bat1)
	suite.claimCardV3(service, mockLedger, w2, w1ProviderID, http.StatusOK, bat1, noUUID())
	suite.CheckBalance(w2, zero)

	suite.CheckBalance(w3, bat1)
	suite.claimCardV3(service, mockLedger, w3, w2ProviderID, http.StatusOK, bat1, noUUID())
	suite.CheckBalance(w3, zero)

	suite.CheckBalance(w4, bat1)
	suite.claimCardV3(service, mockLedger, w4, w3ProviderID, http.StatusConflict, bat1, noUUID())
	suite.CheckBalance(w4, bat1)

	suite.CheckBalance(w3, zero)
	suite.claimCardV3(service, mockLedger, w3, settlement, http.StatusOK, zero, &anonCard2UUID)
	suite.CheckBalance(w3, zero)
}

func (suite *WalletControllersTestSuite) claimCardV3(
	service *Service,
	mockLedgerClient *mockledger.MockClient,
	w *uphold.Wallet,
	destination string,
	status int,
	amount decimal.Decimal,
	anonymousAddress *uuid.UUID,
) (*walletutils.Info, string) {
	signedCreationRequest, err := w.PrepareTransaction(*w.AltCurrency, altcurrency.BAT.ToProbi(amount), destination, "")

	suite.Require().NoError(err, "transaction must be signed client side")

	// V3 Payload
	reqBody := ClaimUpholdWalletRequest{
		SignedCreationRequest: signedCreationRequest,
	}

	if anonymousAddress != nil {
		reqBody.AnonymousAddress = anonymousAddress.String()
	}

	body, err := json.Marshal(&reqBody)
	suite.Require().NoError(err, "unable to marshal claim body")

	info := w.GetWalletInfo()
	mockLedgerClient.EXPECT().GetMemberWallets(gomock.Any(), gomock.Eq(uuid.Must(uuid.FromString(info.ID)))).Return(&[]walletutils.Info{info}, nil)

	// V3 Handler

	handler := ClaimUpholdWalletV3(service)

	req, err := http.NewRequest("POST", "/v3/wallet/{paymentID}/claim", bytes.NewBuffer(body))
	suite.Require().NoError(err, "wallet claim request could not be created")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("paymentID", info.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handlers.AppHandler(handler).ServeHTTP(rr, req)
	suite.Require().Equal(status, rr.Code, fmt.Sprintf("status is expected to match %d: %s", status, rr.Body.String()))
	linked, err := service.Datastore.GetWallet(uuid.Must(uuid.FromString(w.ID)))
	suite.Require().NoError(err, "retrieving the wallet did not cause an error")
	return linked, rr.Body.String()
}

func (suite *WalletControllersTestSuite) claimCard(
	service *Service,
	mockLedgerClient *mockledger.MockClient,
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

	info := w.GetWalletInfo()
	mockLedgerClient.EXPECT().GetMemberWallets(gomock.Any(), gomock.Eq(uuid.Must(uuid.FromString(info.ID)))).Return(&[]walletutils.Info{info}, nil)
	handler := LinkWalletCompat(service)
	req, err := http.NewRequest("POST", "/v1/wallet/{paymentID}/claim", bytes.NewBuffer(body))
	suite.Require().NoError(err, "wallet claim request could not be created")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("paymentID", info.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(status, rr.Code, fmt.Sprintf("status is expected to match %d: %s", status, rr.Body.String()))
	linked, err := service.Datastore.GetWallet(uuid.Must(uuid.FromString(w.ID)))
	suite.Require().NoError(err, "retrieving the wallet did not cause an error")
	return linked, rr.Body.String()
}

func (suite *WalletControllersTestSuite) createBody(
	tx string,
) string {
	reqBody, _ := json.Marshal(UpholdCreationRequest{
		SignedCreationRequest: tx,
	})
	return string(reqBody)
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

	var returnedInfo ResponseV3
	err = json.Unmarshal([]byte(createResp), &returnedInfo)
	suite.Require().NoError(err, "unable to create wallet")
	// returnedInfo := walletutils.Info{
	// 	ID:          uuid.NewV4().String(),
	// 	PublicKey:   publicKeyString,
	// 	Provider:    "uphold",
	// 	AltCurrency: &bat,
	// }
	convertedInfo := responseV3ToInfo(returnedInfo)
	w.Info = *convertedInfo
	fmt.Println("returned info")
	fmt.Printf("%#v\n", returnedInfo)
	fmt.Printf("%#v\n", convertedInfo)
	fmt.Printf("%#v\n", w.Info)
	return w
}

func (suite *WalletControllersTestSuite) TestCreateBraveWalletV3() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres connection")

	service := &Service{
		Datastore: pg,
	}

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

	suite.Assert().JSONEq(`{
		"message":"missing signature: invalid signature: A valid signature MUST have algorithm, keyId, and signature keys",
		"code":400
	}`, notSignedResponse, "field is not valid")

	createResp := suite.createBraveWalletV3(
		service,
		``,
		http.StatusCreated,
		publicKey,
		privKey,
		true,
	)

	var created ResponseV3
	err = json.Unmarshal([]byte(createResp), &created)
	suite.Require().NoError(err, "unable to unmarshal response")

	getResp := suite.getWallet(service, uuid.Must(uuid.FromString(created.PaymentID)), http.StatusOK)

	var gotten ResponseV3
	err = json.Unmarshal([]byte(getResp), &gotten)
	suite.Require().NoError(err, "unable to unmarshal response")
	// does not return wallet provider
	suite.Require().Equal(created, gotten, "the get and create return the same structure")
}

func (suite *WalletControllersTestSuite) TestCreateUpholdWalletV3() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres connection")

	service := &Service{
		Datastore: pg,
	}
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
		"code":400,
		"data": {
			"validationErrors": {
				"signedCreationRequest":"value is required",
				"validation":"failed validation: missing signed creation request"
			}
		},
		"message":"Error validating uphold create wallet request validation errors"
	}`, badFieldResponse, "field is not valid")

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
		"message":"missing signature: invalid signature: A valid signature MUST have algorithm, keyId, and signature keys",
		"code":400
	}`, notSignedResponse, "field is not valid")
}

func (suite *WalletControllersTestSuite) getWallet(
	service *Service,
	paymentId uuid.UUID,
	code int,
) string {
	handler := handlers.AppHandler(GetWalletV3)

	req, err := http.NewRequest("GET", "/v3/wallet/"+paymentId.String(), nil)
	suite.Require().NoError(err, "a request should be created")

	req = req.WithContext(context.WithValue(req.Context(), appctx.DatastoreCTXKey, service.Datastore))
	req = req.WithContext(context.WithValue(req.Context(), appctx.RODatastoreCTXKey, service.Datastore))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("paymentID", paymentId.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Require().Equal(code, rr.Code, "known status code should be sent: "+rr.Body.String())

	return rr.Body.String()
}

func (suite *WalletControllersTestSuite) createBraveWalletV3(
	service *Service,
	body string,
	code int,
	publicKey httpsignature.Ed25519PubKey,
	privateKey ed25519.PrivateKey,
	shouldSign bool,
) string {

	handler := handlers.AppHandler(CreateBraveWalletV3)

	bodyBuffer := bytes.NewBuffer([]byte(body))
	req, err := http.NewRequest("POST", "/v3/wallet/brave", bodyBuffer)
	suite.Require().NoError(err, "a request should be created")

	// setup context
	req = req.WithContext(context.WithValue(context.Background(), appctx.DatastoreCTXKey, service.Datastore))

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
	service *Service,
	body string,
	code int,
	publicKey httpsignature.Ed25519PubKey,
	privateKey ed25519.PrivateKey,
	shouldSign bool,
) string {

	handler := handlers.AppHandler(CreateUpholdWalletV3)

	bodyBuffer := bytes.NewBuffer([]byte(body))
	req, err := http.NewRequest("POST", "/v3/wallet", bodyBuffer)
	suite.Require().NoError(err, "a request should be created")

	// setup context
	req = req.WithContext(context.WithValue(context.Background(), appctx.DatastoreCTXKey, service.Datastore))

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

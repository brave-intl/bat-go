//go:build integration

package wallet

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/services/wallet/storage"

	"github.com/brave-intl/bat-go/libs/clients"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/backoff"
	mockreputation "github.com/brave-intl/bat-go/libs/clients/reputation/mock"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/test"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/services/wallet/wallettest"
	"github.com/go-chi/chi"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
)

type WalletControllersV4TestSuite struct {
	storage Datastore
	suite.Suite
}

func TestWalletControllersV4TestSuite(t *testing.T) {
	suite.Run(t, new(WalletControllersV4TestSuite))
}

func (suite *WalletControllersV4TestSuite) SetupSuite() {
	wallettest.Migrate(suite.T())
	storage, _ := NewWritablePostgres("", false, "")
	suite.storage = storage
}

func (suite *WalletControllersV4TestSuite) SetupTest() {
	wallettest.CleanDB(suite.T(), suite.storage.RawDB())
}

func (suite *WalletControllersV4TestSuite) TestCreateBraveWalletV4_Success() {
	ctx := context.Background()

	storage, err := NewWritablePostgres("", false, "")
	suite.NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	reputationClient := mockreputation.NewMockClient(ctrl)
	reputationClient.EXPECT().
		UpsertReputationSummary(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	geoCountry := "AF"

	locationValidator := NewMockGeoValidator(ctrl)
	locationValidator.EXPECT().
		Validate(gomock.Any(), geoCountry).
		Return(true, nil)

	s := &Service{
		Datastore:    storage,
		repClient:    reputationClient,
		geoValidator: locationValidator,
		retry:        backoff.Retry,
		dappConf:     DAppConfig{},
	}

	router := chi.NewRouter()
	RegisterRoutes(ctx, s, router, noOpInstrumentHandler(), noOpMw(), noOpMw())

	data := V4Request{
		GeoCountry: geoCountry,
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	rw := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPost, "/v4/wallets", bytes.NewBuffer(payload))

	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	err = signRequest(request, publicKey, privateKey)
	suite.Require().NoError(err)

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, request)

	suite.Require().Equal(http.StatusCreated, rw.Code)

	var response V4Response
	err = json.NewDecoder(rw.Body).Decode(&response)
	suite.Require().NoError(err)

	walletID := uuid.NewV5(ClaimNamespace, publicKey.String())
	suite.Assert().Equal(walletID.String(), response.PaymentID)
}

func (suite *WalletControllersV4TestSuite) TestCreateBraveWalletV4_GeoCountryDisabled() {
	ctx := context.Background()

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	locationValidator := NewMockGeoValidator(ctrl)
	locationValidator.EXPECT().
		Validate(gomock.Any(), gomock.Any()).
		Return(false, nil)

	s := &Service{
		geoValidator: locationValidator,
		retry:        backoff.Retry,
		dappConf:     DAppConfig{},
	}

	router := chi.NewRouter()
	RegisterRoutes(ctx, s, router, noOpInstrumentHandler(), noOpMw(), noOpMw())

	data := V4Request{
		GeoCountry: "AF",
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	rw := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPost, "/v4/wallets", bytes.NewBuffer(payload))

	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	err = signRequest(request, publicKey, privateKey)
	suite.Require().NoError(err)

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, request)

	suite.Assert().Equal(http.StatusForbidden, rw.Code)

	walletID := uuid.NewV5(ClaimNamespace, publicKey.String())

	info, err := suite.storage.GetWallet(ctx, walletID)
	suite.Require().NoError(err)

	suite.Assert().Nil(info)
}

func (suite *WalletControllersV4TestSuite) TestCreateBraveWalletV4_WalletAlreadyExists() {
	ctx := context.Background()

	storage, err := NewWritablePostgres("", false, "")
	suite.NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	geoCountry := "AF"

	locationValidator := NewMockGeoValidator(ctrl)
	locationValidator.EXPECT().
		Validate(gomock.Any(), geoCountry).
		Return(true, nil)

	s := &Service{
		Datastore:    storage,
		geoValidator: locationValidator,
		dappConf:     DAppConfig{},
	}

	router := chi.NewRouter()
	RegisterRoutes(ctx, s, router, noOpInstrumentHandler(), noOpMw(), noOpMw())

	data := V4Request{
		GeoCountry: geoCountry,
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	rw := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPost, "/v4/wallets", bytes.NewBuffer(payload))

	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	err = signRequest(request, publicKey, privateKey)
	suite.Require().NoError(err)

	// create existing wallet
	paymentID := uuid.NewV5(ClaimNamespace, publicKey.String()).String()
	var altCurrency = altcurrency.BAT
	info := &walletutils.Info{
		ID:          paymentID,
		Provider:    "brave",
		PublicKey:   publicKey.String(),
		AltCurrency: &altCurrency,
	}

	err = suite.storage.InsertWallet(ctx, info)
	suite.Require().NoError(err)

	// execute request
	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, request)

	suite.Require().Equal(http.StatusConflict, rw.Code)

	var appError handlers.AppError
	err = json.NewDecoder(rw.Body).Decode(&appError)
	suite.Require().NoError(err)

	suite.Assert().Contains(appError.Message, "rewards wallet already exists")
}

func (suite *WalletControllersV4TestSuite) TestCreateBraveWalletV4_ReputationCallFailed() {
	ctx := context.Background()

	storage, err := NewWritablePostgres("", false, "")
	suite.NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	errReputation := errorutils.New(errors.New(test.RandomString()),
		test.RandomString(), test.RandomString())

	reputationClient := mockreputation.NewMockClient(ctrl)
	reputationClient.EXPECT().
		UpsertReputationSummary(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errReputation)

	locationValidator := NewMockGeoValidator(ctrl)
	locationValidator.EXPECT().
		Validate(gomock.Any(), gomock.Any()).
		Return(true, nil)

	s := &Service{Datastore: storage,
		repClient:    reputationClient,
		geoValidator: locationValidator,
		retry:        backoff.Retry,
		dappConf:     DAppConfig{},
	}

	router := chi.NewRouter()
	RegisterRoutes(ctx, s, router, noOpInstrumentHandler(), noOpMw(), noOpMw())

	data := V4Request{
		GeoCountry: "AF",
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	rw := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPost, "/v4/wallets", bytes.NewBuffer(payload))

	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	err = signRequest(request, publicKey, privateKey)
	suite.Require().NoError(err)

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, request)

	suite.Assert().Equal(http.StatusInternalServerError, rw.Code)

	walletID := uuid.NewV5(ClaimNamespace, publicKey.String())

	info, err := suite.storage.GetWallet(ctx, walletID)
	suite.Require().NoError(err)

	suite.Assert().Nil(info)
}

func (suite *WalletControllersV4TestSuite) TestUpdateBraveWalletV4_Success() {
	ctx := context.Background()

	storage, err := NewWritablePostgres("", false, "")
	suite.NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	reputationClient := mockreputation.NewMockClient(ctrl)
	reputationClient.EXPECT().
		UpsertReputationSummary(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	s := &Service{
		Datastore: storage,
		repClient: reputationClient,
		retry:     backoff.Retry,
		dappConf:  DAppConfig{},
	}

	// create rewards wallet with public key
	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	paymentID := uuid.NewV5(ClaimNamespace, publicKey.String()).String()

	var altCurrency = altcurrency.BAT
	info := &walletutils.Info{
		ID:          paymentID,
		Provider:    "brave",
		PublicKey:   publicKey.String(),
		AltCurrency: &altCurrency,
	}

	err = suite.storage.InsertWallet(ctx, info)
	suite.Require().NoError(err)

	router := chi.NewRouter()
	RegisterRoutes(ctx, s, router, noOpInstrumentHandler(), noOpMw(), noOpMw())

	data := V4Request{
		GeoCountry: "AF",
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	rw := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/v4/wallets/%s", paymentID),
		bytes.NewBuffer(payload))

	err = signUpdateRequest(request, paymentID, privateKey)
	suite.Require().NoError(err)

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, request)

	suite.Require().Equal(http.StatusOK, rw.Code)
}

func (suite *WalletControllersV4TestSuite) TestUpdateBraveWalletV4_VerificationMissingWallet() {
	ctx := context.Background()

	storage, err := NewWritablePostgres("", false, "")
	suite.NoError(err)

	s := &Service{
		Datastore: storage,
		retry:     backoff.Retry,
		dappConf:  DAppConfig{},
	}

	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	paymentID := uuid.NewV5(ClaimNamespace, publicKey.String()).String()

	router := chi.NewRouter()
	RegisterRoutes(ctx, s, router, noOpInstrumentHandler(), noOpMw(), noOpMw())

	data := V4Request{
		GeoCountry: "AF",
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	rw := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/v4/wallets/%s", paymentID),
		bytes.NewBuffer(payload))

	err = signUpdateRequest(request, paymentID, privateKey)
	suite.Require().NoError(err)

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, request)

	suite.Require().Equal(http.StatusForbidden, rw.Code)
}

func (suite *WalletControllersV4TestSuite) TestUpdateBraveWalletV4_PaymentIDMismatch() {
	ctx := context.Background()

	storage, err := NewWritablePostgres("", false, "")
	suite.NoError(err)

	s := &Service{
		Datastore: storage,
		retry:     backoff.Retry,
		dappConf:  DAppConfig{},
	}

	// create rewards wallet with public key
	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	paymentID := uuid.NewV5(ClaimNamespace, publicKey.String()).String()

	var altCurrency = altcurrency.BAT
	info := &walletutils.Info{
		ID:          paymentID,
		Provider:    "brave",
		PublicKey:   publicKey.String(),
		AltCurrency: &altCurrency,
	}

	err = suite.storage.InsertWallet(ctx, info)
	suite.Require().NoError(err)

	router := chi.NewRouter()
	RegisterRoutes(ctx, s, router, noOpInstrumentHandler(), noOpMw(), noOpMw())

	data := V4Request{
		GeoCountry: "AF",
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	rw := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/v4/wallets/%s", uuid.NewV4()),
		bytes.NewBuffer(payload))

	err = signUpdateRequest(request, paymentID, privateKey)
	suite.Require().NoError(err)

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, request)

	suite.Require().Equal(http.StatusForbidden, rw.Code)

	var appError handlers.AppError
	err = json.NewDecoder(rw.Body).Decode(&appError)
	suite.Require().NoError(err)

	suite.Assert().Contains(appError.Message, "error payment id does not match http signature key id")
}

func (suite *WalletControllersV4TestSuite) TestUpdateBraveWalletV4_GeoCountryAlreadySet() {
	ctx := context.Background()

	storage, err := NewWritablePostgres("", false, "")
	suite.NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	errorBundle := clients.NewHTTPError(errors.New(test.RandomString()), test.RandomString(),
		test.RandomString(), http.StatusConflict, nil)

	reputationClient := mockreputation.NewMockClient(ctrl)
	reputationClient.EXPECT().
		UpsertReputationSummary(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errorBundle)

	service := &Service{
		Datastore: storage,
		repClient: reputationClient,
		retry:     backoff.Retry,
		dappConf:  DAppConfig{},
	}

	// create rewards wallet with public key
	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	paymentID := uuid.NewV5(ClaimNamespace, publicKey.String()).String()

	var altCurrency = altcurrency.BAT
	info := &walletutils.Info{
		ID:          paymentID,
		Provider:    "brave",
		PublicKey:   publicKey.String(),
		AltCurrency: &altCurrency,
	}

	err = suite.storage.InsertWallet(ctx, info)
	suite.Require().NoError(err)

	router := chi.NewRouter()
	RegisterRoutes(ctx, service, router, noOpInstrumentHandler(), noOpMw(), noOpMw())

	data := V4Request{
		GeoCountry: "AF",
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	rw := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/v4/wallets/%s", paymentID),
		bytes.NewBuffer(payload))

	err = signUpdateRequest(request, paymentID, privateKey)
	suite.Require().NoError(err)

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, request)

	suite.Require().Equal(http.StatusConflict, rw.Code)

	var appError handlers.AppError
	err = json.NewDecoder(rw.Body).Decode(&appError)
	suite.Require().NoError(err)

	suite.Assert().Contains(appError.Error(), "error geo country has already been set for rewards wallet")
}

func (suite *WalletControllersV4TestSuite) TestUpdateBraveWalletV4_ReputationCallFailed() {
	ctx := context.Background()

	storage, err := NewWritablePostgres("", false, "")
	suite.NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	errReputation := errors.New(test.RandomString())
	reputationClient := mockreputation.NewMockClient(ctrl)
	reputationClient.EXPECT().
		UpsertReputationSummary(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errReputation)

	s := &Service{
		Datastore: storage,
		repClient: reputationClient,
		retry:     backoff.Retry,
		dappConf:  DAppConfig{},
	}

	// create rewards wallet with public key
	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	paymentID := uuid.NewV5(ClaimNamespace, publicKey.String()).String()

	var altCurrency = altcurrency.BAT
	info := &walletutils.Info{
		ID:          paymentID,
		Provider:    "brave",
		PublicKey:   publicKey.String(),
		AltCurrency: &altCurrency,
	}

	err = suite.storage.InsertWallet(ctx, info)
	suite.Require().NoError(err)

	router := chi.NewRouter()
	RegisterRoutes(ctx, s, router, noOpInstrumentHandler(), noOpMw(), noOpMw())

	data := V4Request{
		GeoCountry: "AF",
	}

	payload, err := json.Marshal(data)
	suite.Require().NoError(err)

	rw := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/v4/wallets/%s", paymentID),
		bytes.NewBuffer(payload))

	err = signUpdateRequest(request, paymentID, privateKey)
	suite.Require().NoError(err)

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, request)

	suite.Require().Equal(http.StatusInternalServerError, rw.Code)
}

func (suite *WalletControllersTestSuite) TestGetWalletV4() {
	pg, _, err := NewPostgres()
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

	service := &Service{
		Datastore:     pg,
		allowListRepo: allowList,
		dappConf:      DAppConfig{},
	}

	handler := handlers.AppHandler(GetWalletV4(service))

	req, err := http.NewRequest("GET", "/v4/wallets/"+paymentID.String(), nil)
	suite.Require().NoError(err, "a request should be created")

	req = req.WithContext(context.WithValue(req.Context(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("paymentID", paymentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Assert().Equal(http.StatusOK, rr.Code)

	var resp ResponseV4
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	suite.Require().NoError(err)

	suite.Assert().Equal(true, resp.SelfCustodyAvailable["solana"])
}

func (suite *WalletControllersTestSuite) TestGetWalletV4_Not_Whitelisted() {
	pg, _, err := NewPostgres()
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

	allowList := storage.NewAllowList()

	s := &Service{
		Datastore:     pg,
		allowListRepo: allowList,
		dappConf:      DAppConfig{},
	}

	handler := handlers.AppHandler(GetWalletV4(s))

	req, err := http.NewRequest("GET", "/v4/wallets/"+paymentID.String(), nil)
	suite.Require().NoError(err, "a request should be created")

	req = req.WithContext(context.WithValue(req.Context(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("paymentID", paymentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Assert().Equal(http.StatusOK, rr.Code)

	var resp ResponseV4
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	suite.Require().NoError(err)

	suite.Assert().Equal(false, resp.SelfCustodyAvailable["solana"])
}

func signUpdateRequest(req *http.Request, paymentID string, privateKey ed25519.PrivateKey) error {
	var s httpsignature.SignatureParams
	s.Algorithm = httpsignature.ED25519
	s.KeyID = paymentID
	s.Headers = []string{"digest", "(request-target)"}
	return s.Sign(privateKey, crypto.Hash(0), req)
}

func noOpInstrumentHandler() middleware.InstrumentHandlerDef {
	return func(name string, h http.Handler) http.Handler {
		return h
	}
}

func noOpMw() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
}

//go:build integration

package wallet_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brave-intl/bat-go/libs/clients"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/backoff"
	mockreputation "github.com/brave-intl/bat-go/libs/clients/reputation/mock"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/test"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/services/wallet"
	"github.com/brave-intl/bat-go/services/wallet/wallettest"
	"github.com/go-chi/chi"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
)

type WalletControllersV4TestSuite struct {
	storage wallet.Datastore
	suite.Suite
}

func TestWalletControllersV4TestSuite(t *testing.T) {
	suite.Run(t, new(WalletControllersV4TestSuite))
}

func (suite *WalletControllersV4TestSuite) SetupSuite() {
	wallettest.Migrate(suite.T())
	storage, _ := wallet.NewWritablePostgres("", false, "")
	suite.storage = storage
}

func (suite *WalletControllersV4TestSuite) SetupTest() {
	wallettest.CleanDB(suite.T(), suite.storage.RawDB())
}

func (suite *WalletControllersV4TestSuite) TestCreateBraveWalletV4_Success() {
	ctx := context.Background()

	storage, err := wallet.NewWritablePostgres("", false, "")
	suite.NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	reputationClient := mockreputation.NewMockClient(ctrl)
	reputationClient.EXPECT().
		UpsertReputationSummary(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	geoCountry := "AF"

	locationValidator := wallet.NewMockGeoValidator(ctrl)
	locationValidator.EXPECT().
		Validate(gomock.Any(), geoCountry).
		Return(true, nil)

	service, err := wallet.InitService(storage, nil, reputationClient, nil, locationValidator, backoff.Retry)
	suite.Require().NoError(err)

	router := chi.NewRouter()
	wallet.RegisterRoutes(ctx, service, router)

	data := wallet.V4Request{
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

	var response wallet.V4Response
	err = json.NewDecoder(rw.Body).Decode(&response)
	suite.Require().NoError(err)

	walletID := uuid.NewV5(wallet.ClaimNamespace, publicKey.String())
	suite.Assert().Equal(walletID.String(), response.PaymentID)
}

func (suite *WalletControllersV4TestSuite) TestCreateBraveWalletV4_GeoCountryDisabled() {
	ctx := context.Background()

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	locationValidator := wallet.NewMockGeoValidator(ctrl)
	locationValidator.EXPECT().
		Validate(gomock.Any(), gomock.Any()).
		Return(false, nil)

	service, err := wallet.InitService(nil, nil, nil, nil, locationValidator, backoff.Retry)
	suite.Require().NoError(err)

	router := chi.NewRouter()
	wallet.RegisterRoutes(ctx, service, router)

	data := wallet.V4Request{
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

	walletID := uuid.NewV5(wallet.ClaimNamespace, publicKey.String())

	info, err := suite.storage.GetWallet(ctx, walletID)
	suite.Require().NoError(err)

	suite.Assert().Nil(info)
}

func (suite *WalletControllersV4TestSuite) TestCreateBraveWalletV4_WalletAlreadyExists() {
	ctx := context.Background()

	storage, err := wallet.NewWritablePostgres("", false, "")
	suite.NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	geoCountry := "AF"

	locationValidator := wallet.NewMockGeoValidator(ctrl)
	locationValidator.EXPECT().
		Validate(gomock.Any(), geoCountry).
		Return(true, nil)

	service, err := wallet.InitService(storage, nil, nil, nil, locationValidator, nil)
	suite.Require().NoError(err)

	router := chi.NewRouter()
	wallet.RegisterRoutes(ctx, service, router)

	data := wallet.V4Request{
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
	paymentID := uuid.NewV5(wallet.ClaimNamespace, publicKey.String()).String()
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

	storage, err := wallet.NewWritablePostgres("", false, "")
	suite.NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	errReputation := errorutils.New(errors.New(test.RandomString()),
		test.RandomString(), test.RandomString())

	reputationClient := mockreputation.NewMockClient(ctrl)
	reputationClient.EXPECT().
		UpsertReputationSummary(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errReputation)

	locationValidator := wallet.NewMockGeoValidator(ctrl)
	locationValidator.EXPECT().
		Validate(gomock.Any(), gomock.Any()).
		Return(true, nil)

	service, err := wallet.InitService(storage, nil, reputationClient, nil, locationValidator, backoff.Retry)
	suite.Require().NoError(err)

	router := chi.NewRouter()
	wallet.RegisterRoutes(ctx, service, router)

	data := wallet.V4Request{
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

	walletID := uuid.NewV5(wallet.ClaimNamespace, publicKey.String())

	info, err := suite.storage.GetWallet(ctx, walletID)
	suite.Require().NoError(err)

	suite.Assert().Nil(info)
}

func (suite *WalletControllersV4TestSuite) TestUpdateBraveWalletV4_Success() {
	ctx := context.Background()

	storage, err := wallet.NewWritablePostgres("", false, "")
	suite.NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	reputationClient := mockreputation.NewMockClient(ctrl)
	reputationClient.EXPECT().
		UpsertReputationSummary(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	service, err := wallet.InitService(storage, nil, reputationClient, nil,
		nil, backoff.Retry)
	suite.Require().NoError(err)

	// create rewards wallet with public key
	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	paymentID := uuid.NewV5(wallet.ClaimNamespace, publicKey.String()).String()

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
	wallet.RegisterRoutes(ctx, service, router)

	data := wallet.V4Request{
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

	storage, err := wallet.NewWritablePostgres("", false, "")
	suite.NoError(err)

	service, err := wallet.InitService(storage, nil, nil, nil,
		nil, backoff.Retry)
	suite.Require().NoError(err)

	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	paymentID := uuid.NewV5(wallet.ClaimNamespace, publicKey.String()).String()

	router := chi.NewRouter()
	wallet.RegisterRoutes(ctx, service, router)

	data := wallet.V4Request{
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

	storage, err := wallet.NewWritablePostgres("", false, "")
	suite.NoError(err)

	service, err := wallet.InitService(storage, nil, nil, nil,
		nil, backoff.Retry)
	suite.Require().NoError(err)

	// create rewards wallet with public key
	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	paymentID := uuid.NewV5(wallet.ClaimNamespace, publicKey.String()).String()

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
	wallet.RegisterRoutes(ctx, service, router)

	data := wallet.V4Request{
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

	storage, err := wallet.NewWritablePostgres("", false, "")
	suite.NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	errorBundle := clients.NewHTTPError(errors.New(test.RandomString()), test.RandomString(),
		test.RandomString(), http.StatusConflict, nil)

	reputationClient := mockreputation.NewMockClient(ctrl)
	reputationClient.EXPECT().
		UpsertReputationSummary(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errorBundle)

	service, err := wallet.InitService(storage, nil, reputationClient, nil,
		nil, backoff.Retry)
	suite.Require().NoError(err)

	// create rewards wallet with public key
	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	paymentID := uuid.NewV5(wallet.ClaimNamespace, publicKey.String()).String()

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
	wallet.RegisterRoutes(ctx, service, router)

	data := wallet.V4Request{
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

	storage, err := wallet.NewWritablePostgres("", false, "")
	suite.NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	errReputation := errors.New(test.RandomString())
	reputationClient := mockreputation.NewMockClient(ctrl)
	reputationClient.EXPECT().
		UpsertReputationSummary(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errReputation)

	service, err := wallet.InitService(storage, nil, reputationClient, nil,
		nil, backoff.Retry)
	suite.Require().NoError(err)

	// create rewards wallet with public key
	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	paymentID := uuid.NewV5(wallet.ClaimNamespace, publicKey.String()).String()

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
	wallet.RegisterRoutes(ctx, service, router)

	data := wallet.V4Request{
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

func signUpdateRequest(req *http.Request, paymentID string, privateKey ed25519.PrivateKey) error {
	var s httpsignature.SignatureParams
	s.Algorithm = httpsignature.ED25519
	s.KeyID = paymentID
	s.Headers = []string{"digest", "(request-target)"}
	return s.Sign(privateKey, crypto.Hash(0), req)
}

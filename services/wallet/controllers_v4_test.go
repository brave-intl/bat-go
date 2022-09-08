//go:build integration

package wallet_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	mockreputation "github.com/brave-intl/bat-go/libs/clients/reputation/mock"
	"github.com/brave-intl/bat-go/libs/test"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/libs/httpsignature"
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
	wallet.ReputationGeoEnable = true

	ctx := context.Background()

	storage, err := wallet.NewWritablePostgres("", false, "")
	suite.NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	reputationClient := mockreputation.NewMockClient(ctrl)
	reputationClient.EXPECT().
		CreateReputationSummary(gomock.Any(), gomock.Any(), gomock.Any()).
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

	data := wallet.CreateWalletV4Request{
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

	var info walletutils.Info
	err = json.NewDecoder(rw.Body).Decode(&info)
	suite.Require().NoError(err)

	walletID := uuid.NewV5(wallet.ClaimNamespace, publicKey.String())
	suite.Assert().Equal(walletID.String(), info.ID)
}

func (suite *WalletControllersV4TestSuite) TestCreateBraveWalletV4_GeoCountryDisabled() {
	wallet.ReputationGeoEnable = true

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

	data := wallet.CreateWalletV4Request{
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

func (suite *WalletControllersV4TestSuite) TestCreateBraveWalletV4_ReputationCallFailed() {
	wallet.ReputationGeoEnable = true

	ctx := context.Background()

	storage, err := wallet.NewWritablePostgres("", false, "")
	suite.NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	errReputation := errors.New(test.RandomString())
	reputationClient := mockreputation.NewMockClient(ctrl)
	reputationClient.EXPECT().
		CreateReputationSummary(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errReputation)

	locationValidator := wallet.NewMockGeoValidator(ctrl)
	locationValidator.EXPECT().
		Validate(gomock.Any(), gomock.Any()).
		Return(true, nil)

	service, err := wallet.InitService(storage, nil, reputationClient, nil, locationValidator, backoff.Retry)
	suite.Require().NoError(err)

	router := chi.NewRouter()
	wallet.RegisterRoutes(ctx, service, router)

	data := wallet.CreateWalletV4Request{
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

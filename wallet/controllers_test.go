// +build integration

package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
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

func (suite *ControllersTestSuite) SetupTest() {
	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions"}

	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.DB.Exec("delete from " + table)
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func (suite *ControllersTestSuite) TestLinkWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres connection")

	service := &Service{
		datastore: pg,
	}
	providerLinkingID := uuid.NewV4()
	secondProviderLinkingID := uuid.NewV4()
	nilUUID := noUUID()

	w := suite.CreateWallet(service)
	linkRequest := suite.CreateWalletLinkPayload(providerLinkingID)
	wallet, _ := suite.WalletLinkDo(service, w, linkRequest, http.StatusOK)
	suite.Require().Equal(linkRequest.ProviderLinkingID, wallet.ProviderLinkingID, "provider linking id was not linked")
	suite.Require().Equal(linkRequest.AnonymousAddress, wallet.AnonymousAddress, "anonymous address was not linked")

	linkRequest = suite.CreateWalletLinkPayload(providerLinkingID)
	wallet, _ = suite.WalletLinkDo(service, w, linkRequest, http.StatusOK)
	suite.Require().Equal(linkRequest.ProviderLinkingID, wallet.ProviderLinkingID, "provider linking id was not linked")
	suite.Require().Equal(linkRequest.AnonymousAddress, wallet.AnonymousAddress, "anonymous address was not linked")

	previousLinkRequest := linkRequest
	linkRequest = suite.CreateWalletLinkPayload(secondProviderLinkingID)
	wallet, _ = suite.WalletLinkDo(service, w, linkRequest, http.StatusConflict)
	suite.Require().Equal(previousLinkRequest.ProviderLinkingID, wallet.ProviderLinkingID, "provider linking id was not linked")
	suite.Require().Equal(previousLinkRequest.AnonymousAddress, wallet.AnonymousAddress, "anonymous address was not linked")

	w = suite.CreateWallet(service)
	linkRequest = suite.CreateWalletLinkPayload(providerLinkingID)
	wallet, _ = suite.WalletLinkDo(service, w, linkRequest, http.StatusOK)
	suite.Require().Equal(linkRequest.ProviderLinkingID, wallet.ProviderLinkingID, "provider linking id was not linked")
	suite.Require().Equal(linkRequest.AnonymousAddress, wallet.AnonymousAddress, "anonymous address was not linked")

	w = suite.CreateWallet(service)
	linkRequest = suite.CreateWalletLinkPayload(providerLinkingID)
	wallet, _ = suite.WalletLinkDo(service, w, linkRequest, http.StatusOK)
	suite.Require().Equal(linkRequest.ProviderLinkingID, wallet.ProviderLinkingID, "provider linking id was not linked")
	suite.Require().Equal(linkRequest.AnonymousAddress, wallet.AnonymousAddress, "anonymous address was not linked")

	w = suite.CreateWallet(service)
	linkRequest = suite.CreateWalletLinkPayload(providerLinkingID)
	wallet, _ = suite.WalletLinkDo(service, w, linkRequest, http.StatusInternalServerError)
	suite.Require().Equal(nilUUID, wallet.ProviderLinkingID, "provider linking id was not linked")
	suite.Require().Equal(nilUUID, wallet.AnonymousAddress, "anonymous address was not linked")
}

func noUUID() *uuid.UUID {
	return nil
}

func (suite *ControllersTestSuite) WalletLinkDo(
	service *Service,
	w *Info,
	linkRequest *LinkWalletRequest,
	status int,
) (*Info, string) {
	handler := PostLinkWalletCompat(service)

	body, err := json.Marshal(linkRequest)
	suite.Require().NoError(err, "could not marshal claim request")

	req, err := http.NewRequest("POST", "/v1/wallet/{paymentID}/link", bytes.NewBuffer(body))
	suite.Require().NoError(err, "wallet claim request could not be created")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("paymentID", w.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(status, rr.Code, fmt.Sprintf("status is expected to match %d", status))
	wallet, err := service.datastore.GetWallet(uuid.Must(uuid.FromString(w.ID)))
	suite.Require().NoError(err, "retrieving the wallet did not cause an error")
	return wallet, rr.Body.String()
}

func (suite *ControllersTestSuite) CreateWalletLinkPayload(providerLinkingID uuid.UUID) *LinkWalletRequest {
	anonymousAddress := uuid.NewV4()
	return &LinkWalletRequest{
		ProviderLinkingID: &providerLinkingID,
		AnonymousAddress:  &anonymousAddress,
	}
}

func (suite *ControllersTestSuite) CreateWallet(service *Service) *Info {
	walletID := uuid.NewV4().String()
	w := &Info{
		ID:         walletID,
		Provider:   "uphold",
		ProviderID: uuid.NewV4().String(),
		PublicKey:  "-",
	}
	suite.Require().NoError(service.datastore.InsertWallet(w), "wallet should be insertable")
	return w
}

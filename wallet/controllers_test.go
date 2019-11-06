// +build integration

package wallet

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/ed25519"
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
		_, err := pg.DB.Exec("delete from " + table + " returning *")
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func (suite *WalletControllersTestSuite) TestPostCreateWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres connection")
	service := &Service{
		datastore: pg,
	}

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	publicKeyString := hex.EncodeToString(publicKey)
	createBody := func(provider string, publicKey string) string {
		return `{"provider":"` + provider + `","publicKey":"` + publicKey + `"}`
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
		"message": "Error unmarshalling body: unexpected end of JSON input",
		"code": 400
	}`, badJSONBodyParse, "should fail when parsing json")

	badFieldResponse := suite.createWallet(
		service,
		createBody("uphold", publicKeyString),
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
				"provider": "'uphold' is not supported"
			}
		}
	}`, badFieldResponse, "field is not valid")

	// assume 403 is already covered
	// fail because of lacking signature presence
	notSignedResponse := suite.createWallet(
		service,
		createBody("client", publicKeyString),
		http.StatusBadRequest,
		publicKey,
		privKey,
		false,
	)
	suite.Assert().Equal(`Bad Request
`, notSignedResponse, "not signed creation requests should fail")
	// body public key does not match signature public key
	notMatchingResponse := suite.createWallet(
		service,
		createBody("client", uuid.NewV4().String()),
		http.StatusBadRequest,
		publicKey,
		privKey,
		true,
	)
	suite.Assert().JSONEq(`{
		"message": "Error validating request signature",
		"code": 400,
		"data": {
			"validationErrors": {
				"publicKey": "publicKey must match signature"
			}
		}
	}`, notMatchingResponse, "body should not match keyId")
	createResp := suite.createWallet(
		service,
		createBody("client", publicKeyString),
		http.StatusCreated,
		publicKey,
		privKey,
		true,
	)

	var created Info
	err = json.Unmarshal([]byte(createResp), &created)
	suite.Require().NoError(err, "unable to unmarshal response")

	getResp := suite.getWallet(service, created.ID, http.StatusOK)

	var gotten Info
	err = json.Unmarshal([]byte(getResp), &gotten)
	suite.Require().NoError(err, "unable to unmarshal response")
	// gotten.PrivateKey = created.PrivateKey
	suite.Assert().Equal(created, gotten, "the get and create return the same structure")
}

func (suite *WalletControllersTestSuite) getWallet(
	service *Service,
	paymentId string,
	code int,
) string {
	handler := GetWallet(service)

	req, err := http.NewRequest("GET", "/v1/wallet/"+paymentId, nil)
	suite.Require().NoError(err, "a request should be created")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("paymentId", paymentId)
	joined := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	req = req.WithContext(joined)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Assert().Equal(code, rr.Code, "known status code should be sent")

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
	suite.Assert().Equal(code, rr.Code, "known status code should be sent")

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

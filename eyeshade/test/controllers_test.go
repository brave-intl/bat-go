// +build eyeshade

package test

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/eyeshade/datastore"
	eyeshade "github.com/brave-intl/bat-go/eyeshade/service"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
)

type ControllersSuite struct {
	suite.Suite
	tokens  map[string]string
	ctx     context.Context
	db      datastore.Datastore
	rodb    datastore.Datastore
	mock    sqlmock.Sqlmock
	mockRO  sqlmock.Sqlmock
	server  *httptest.Server
	service *eyeshade.Service
}

func TestControllersSuite(t *testing.T) {
	suite.Run(t, new(ControllersSuite))
}

func (suite *ControllersSuite) SetupSuite() {
	ctx := context.Background()
	// setup mock DB we will inject into our pg
	mockDB, mock, err := sqlmock.New()
	suite.Require().NoError(err, "failed to create a sql mock")
	mockRODB, mockRO, err := sqlmock.New()
	suite.Require().NoError(err, "failed to create a sql mock")

	suite.ctx = ctx
	suite.db = datastore.NewFromConnection(sqlx.NewDb(mockDB, "sqlmock"))
	suite.mock = mock
	suite.rodb = datastore.NewFromConnection(sqlx.NewDb(mockRODB, "sqlmock"))
	suite.mockRO = mockRO

	suite.tokens = map[string]string{
		"publishers": uuid.NewV4().String(),
		"referrals":  uuid.NewV4().String(),
		"global":     uuid.NewV4().String(),
	}
	for key, value := range suite.tokens {
		os.Setenv(middleware.ScopesToEnv[key], value)
	}
	service, err := eyeshade.SetupService(
		eyeshade.WithContext(suite.ctx),
		eyeshade.WithBuildInfo,
		eyeshade.WithNewLogger,
		eyeshade.WithConnections(suite.db, suite.rodb),
		eyeshade.WithNewRouter,
		eyeshade.WithMiddleware,
		eyeshade.WithRoutes,
	)
	suite.Require().NoError(err)
	suite.service = service
	server := httptest.NewServer(service.Router())
	suite.server = server
}

func (suite *ControllersSuite) TearDownSuite() {
	defer suite.server.Close()
}

func (suite *ControllersSuite) DoRequest(
	method string,
	path string,
	body io.Reader,
	token string,
) (*http.Response, []byte) {
	req, err := http.NewRequest(method, suite.server.URL+path, body)
	suite.Require().NoError(err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	suite.Require().NoError(err)
	respBody, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	suite.Require().NoError(err)
	return resp, respBody
}

// +build integration

package eyeshade

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/datastore/grantserver"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/go-chi/chi"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/suite"
)

type ControllersTestSuite struct {
	suite.Suite
	ctx     context.Context
	db      Datastore
	rodb    Datastore
	mock    sqlmock.Sqlmock
	mockRO  sqlmock.Sqlmock
	server  *httptest.Server
	service *Service
	router  chi.Router
}

func TestControllersTestSuite(t *testing.T) {
	suite.Run(t, new(ControllersTestSuite))
}

func (suite *ControllersTestSuite) SetupSuite() {
	ctx := context.Background()
	// setup mock DB we will inject into our pg
	mockDB, mock, err := sqlmock.New()
	suite.Require().NoError(err, "failed to create a sql mock")
	mockRODB, mockRO, err := sqlmock.New()
	suite.Require().NoError(err, "failed to create a sql mock")

	suite.ctx = ctx
	name := "sqlmock"
	suite.db = NewFromConnection(&grantserver.Postgres{
		DB: sqlx.NewDb(mockDB, name),
	}, name)
	suite.mock = mock
	suite.rodb = NewFromConnection(&grantserver.Postgres{
		DB: sqlx.NewDb(mockRODB, name),
	}, name)
	suite.mock = mock
	suite.mockRO = mockRO
	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, suite.db)
	ctx = context.WithValue(ctx, appctx.RODatastoreCTXKey, suite.rodb)

	r, service, err := SetupService(ctx)
	suite.Require().NoError(err)
	suite.service = service
	server := httptest.NewServer(r)
	suite.server = server
	suite.router = r
}

func (suite *ControllersTestSuite) TearDownSuite() {
	defer suite.server.Close()
}

func (suite *ControllersTestSuite) DoRequest(method string, path string, body io.Reader) (*http.Response, []byte) {
	req, err := http.NewRequest(method, suite.server.URL+path, body)
	suite.Require().NoError(err)
	resp, err := http.DefaultClient.Do(req)
	suite.Require().NoError(err)
	respBody, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	suite.Require().NoError(err)
	return resp, respBody
}

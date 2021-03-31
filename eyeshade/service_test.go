// +build integration

package eyeshade

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
)

type ServiceMockTestSuite struct {
	suite.Suite
	ctx     context.Context
	db      Datastore
	rodb    Datastore
	mock    sqlmock.Sqlmock
	mockRO  sqlmock.Sqlmock
	service *Service
}

// type DatastoreTestSuite struct {
// 	suite.Suite
// 	ctx context.Context
// 	db  Datastore
// }

func TestServiceMockTestSuite(t *testing.T) {
	suite.Run(t, new(ServiceMockTestSuite))
	// if os.Getenv("EYESHADE_DB_URL") != "" {
	// 	suite.Run(t, new(DatastoreTestSuite))
	// }
}

func (suite *ServiceMockTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	name := "sqlmock"
	// setup mock DB we will inject into our pg
	mockDB, mock, err := sqlmock.New()
	suite.Require().NoError(err, "failed to create a sql mock")
	mockRODB, mockRO, err := sqlmock.New()
	suite.Require().NoError(err, "failed to create a sql mock")

	suite.db = NewFromConnection(&grantserver.Postgres{
		DB: sqlx.NewDb(mockDB, name),
	}, name)
	suite.mock = mock
	suite.rodb = NewFromConnection(&grantserver.Postgres{
		DB: sqlx.NewDb(mockRODB, name),
	}, name)
	suite.mockRO = mockRO

	service, err := SetupService(
		WithConnections(suite.db, suite.rodb),
	)
	suite.Require().NoError(err)
	suite.service = service
}

func (suite ServiceMockTestSuite) TestGetBalances() {
	accountIDs := []string{
		uuid.NewV4().String(),
		uuid.NewV4().String(),
	}

	expectedBalances := SetupMockGetBalances(
		suite.mockRO,
		accountIDs,
	)
	expectedPending := SetupMockGetPending(
		suite.mockRO,
		accountIDs,
	)
	expected := mergeVotes(expectedPending, expectedBalances)

	balances, err := suite.service.Balances(
		suite.ctx,
		accountIDs,
		true,
	)
	suite.Require().NoError(err)
	expectedMarshalled, err := json.Marshal(expected)
	suite.Require().NoError(err)
	balancesMarshalled, err := json.Marshal(balances)
	suite.Require().NoError(err)

	suite.Require().JSONEq(
		string(expectedMarshalled),
		string(balancesMarshalled),
	)
}

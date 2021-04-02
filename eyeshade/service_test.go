// +build integration

package eyeshade

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/jmoiron/sqlx"
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

func (suite *ServiceMockTestSuite) TestGetBalances() {
	accountIDs := CreateIDs(2)

	expected := suite.SetupMockBalances(accountIDs, accountIDs)
	balances := suite.Balances(accountIDs, true)
	suite.Require().Len(*expected, len(accountIDs))
	suite.Require().Len(*balances, len(*expected))

	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expected),
		MustMarshal(suite.Require(), balances),
	)

	expected = suite.SetupMockBalances(accountIDs)
	balances = suite.Balances(accountIDs, false)
	suite.Require().Len(*expected, len(accountIDs))
	suite.Require().Len(*balances, len(*expected))

	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expected),
		MustMarshal(suite.Require(), balances),
	)
}

func (suite *ServiceMockTestSuite) SetupMockBalances(
	balanceAccountIDs []string,
	pendingAccountIDs ...[]string,
) *[]Balance {
	expectedBalances := SetupMockGetBalances(
		suite.mockRO,
		balanceAccountIDs,
	)
	if len(pendingAccountIDs) == 0 {
		return &expectedBalances
	}

	expectedPending := SetupMockGetPending(
		suite.mockRO,
		pendingAccountIDs[0],
	)
	return mergePendingTransactions(expectedPending, expectedBalances)
}

func (suite *ServiceMockTestSuite) Balances(
	accountIDs []string,
	includePending bool,
) *[]Balance {
	balances, err := suite.service.Balances(
		suite.ctx,
		accountIDs,
		includePending,
	)
	suite.Require().NoError(err)
	return balances
}

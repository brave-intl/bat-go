// +build integration

package test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/eyeshade"
	"github.com/brave-intl/bat-go/eyeshade/datastore"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/eyeshade/must"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/suite"
)

type ServiceMockTestSuite struct {
	suite.Suite
	ctx     context.Context
	db      datastore.Datastore
	rodb    datastore.Datastore
	mock    sqlmock.Sqlmock
	mockRO  sqlmock.Sqlmock
	service *eyeshade.Service
}

func TestServiceMockTestSuite(t *testing.T) {
	suite.Run(t, new(ServiceMockTestSuite))
}

func (suite *ServiceMockTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	name := "sqlmock"
	// setup mock DB we will inject into our pg
	mockDB, mock, err := sqlmock.New()
	suite.Require().NoError(err, "failed to create a sql mock")
	mockRODB, mockRO, err := sqlmock.New()
	suite.Require().NoError(err, "failed to create a sql mock")

	suite.db = datastore.NewFromConnection(&grantserver.Postgres{
		DB: sqlx.NewDb(mockDB, name),
	}, name)
	suite.mock = mock
	suite.rodb = datastore.NewFromConnection(&grantserver.Postgres{
		DB: sqlx.NewDb(mockRODB, name),
	}, name)
	suite.mockRO = mockRO

	service, err := eyeshade.SetupService(
		eyeshade.WithContext(suite.ctx),
		eyeshade.WithConnections(suite.db, suite.rodb),
	)
	suite.Require().NoError(err)
	suite.service = service
}

func (suite *ServiceMockTestSuite) TestGetBalances() {
	accountIDs := must.UUIDsToString(must.RandomIDs(2, true)...)

	expected := suite.SetupMockBalances(accountIDs, accountIDs)
	balances := suite.GetBalances(accountIDs, true)
	suite.Require().Len(*expected, len(accountIDs))
	suite.Require().Len(*balances, len(*expected))

	suite.Require().JSONEq(
		must.Marshal(suite.Require(), expected),
		must.Marshal(suite.Require(), balances),
	)

	expected = suite.SetupMockBalances(accountIDs)
	balances = suite.GetBalances(accountIDs, false)
	suite.Require().Len(*expected, len(accountIDs))
	suite.Require().Len(*balances, len(*expected))

	suite.Require().JSONEq(
		must.Marshal(suite.Require(), expected),
		must.Marshal(suite.Require(), balances),
	)
}

func (suite *ServiceMockTestSuite) SetupMockBalances(
	balanceAccountIDs []string,
	pendingAccountIDs ...[]string,
) *[]models.Balance {
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
	return models.MergePendingTransactions(expectedPending, expectedBalances)
}

func (suite *ServiceMockTestSuite) GetBalances(
	accountIDs []string,
	includePending bool,
) *[]models.Balance {
	balances, err := suite.service.GetBalances(
		suite.ctx,
		accountIDs,
		includePending,
	)
	suite.Require().NoError(err)
	return balances
}

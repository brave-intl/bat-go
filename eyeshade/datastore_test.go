package eyeshade

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type DatastoreMockTestSuite struct {
	suite.Suite
	ctx  context.Context
	db   Datastore
	mock sqlmock.Sqlmock
}

// type DatastoreTestSuite struct {
// 	suite.Suite
// 	ctx context.Context
// 	db  Datastore
// }

func TestDatastoreMockTestSuite(t *testing.T) {
	suite.Run(t, new(DatastoreMockTestSuite))
	// if os.Getenv("EYESHADE_DB_URL") != "" {
	// 	suite.Run(t, new(DatastoreTestSuite))
	// }
}

func (suite *DatastoreMockTestSuite) SetupSuite() {
	ctx := context.Background()
	// setup mock DB we will inject into our pg
	mockDB, mock, err := sqlmock.New()
	suite.Require().NoError(err, "failed to create a sql mock")

	name := "sqlmock"
	suite.db = NewFromConnection(&grantserver.Postgres{
		DB: sqlx.NewDb(mockDB, name),
	}, name)
	suite.ctx = ctx
	suite.mock = mock
}

func SetupMockGetAccountEarnings(
	mock sqlmock.Sqlmock,
	options AccountEarningsOptions,
) []AccountEarnings {
	getRows := sqlmock.NewRows(
		[]string{"channel", "earnings", "account_id"},
	)
	rows := []AccountEarnings{}
	for i := 0; i < options.Limit; i++ {
		accountID := fmt.Sprintf("publishers#uuid:%s", uuid.NewV4().String())
		earnings := decimal.NewFromFloat(
			float64(rand.Intn(100)),
		).Div(
			decimal.NewFromFloat(10),
		)
		channel := uuid.NewV4().String()
		rows = append(rows, AccountEarnings{channel, earnings, accountID})
		// append sql result rows
		getRows = getRows.AddRow(
			channel,
			earnings,
			accountID,
		)
	}
	mock.ExpectQuery(`
select
	channel,
	(.+) as earnings,
	account_id
from account_transactions
where account_type = 'owner'
	and transaction_type = (.+)
group by (.+)
order by earnings (.+)
limit (.+)`).
		WithArgs(options.Type, options.Limit).
		WillReturnRows(getRows)
	return rows
}

func (suite *DatastoreMockTestSuite) TestGetAccountEarnings() {
	options := AccountEarningsOptions{
		Limit:     5,
		Ascending: true,
		Type:      "contributions",
	}
	expecting := SetupMockGetAccountEarnings(suite.mock, options)
	earnings, err := suite.db.GetAccountEarnings(
		suite.ctx,
		options,
	)
	suite.Require().NoError(err)
	suite.Require().Len(*earnings, options.Limit)

	expectingMarshalled, err := json.Marshal(expecting)
	suite.Require().NoError(err)
	earningsMarshalled, err := json.Marshal(earnings)
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(expectingMarshalled), string(earningsMarshalled))
}

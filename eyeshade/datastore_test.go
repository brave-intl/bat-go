package eyeshade

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

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
		WithArgs(options.Type[:len(options.Type)-1], options.Limit).
		WillReturnRows(getRows)
	return rows
}

func SetupMockGetAccountSettlementEarnings(
	mock sqlmock.Sqlmock,
	options AccountSettlementEarningsOptions,
) []AccountSettlementEarnings {
	getRows := sqlmock.NewRows(
		[]string{"channel", "paid", "account_id"},
	)
	rows := []AccountSettlementEarnings{}
	args := []driver.Value{
		fmt.Sprintf("%s_settlement", options.Type[:len(options.Type)-1]),
		options.Limit,
	}
	targetTime := options.StartDate
	if targetTime == nil {
		now := time.Now()
		targetTime = &now
	} else {
		args = append(args, targetTime)
	}
	untilDate := options.UntilDate
	if untilDate == nil {
		untilDatePrep := targetTime.Add(time.Hour * 24 * time.Duration(options.Limit))
		untilDate = &untilDatePrep
	} else {
		args = append(args, untilDate)
	}
	for i := 0; i < options.Limit; i++ {
		accountID := fmt.Sprintf("publishers#uuid:%s", uuid.NewV4().String())
		paid := decimal.NewFromFloat(
			float64(rand.Intn(100)),
		).Div(
			decimal.NewFromFloat(10),
		)
		channel := uuid.NewV4().String()

		if untilDate.Before(targetTime.Add(time.Duration(i) * time.Hour * 24)) {
			break
		}
		rows = append(rows, AccountSettlementEarnings{channel, paid, accountID})
		// append sql result rows
		getRows = getRows.AddRow(
			channel,
			paid,
			accountID,
		)
	}
	mock.ExpectQuery(`
select
	channel,
	(.+) as paid,
	account_id
from account_transactions
where (.+)
group by (.+)
order by paid (.+)
limit (.+)`).
		WithArgs(args...).
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
func (suite *DatastoreMockTestSuite) TestGetAccountSettlementEarnings() {
	options := AccountSettlementEarningsOptions{
		Limit:     5,
		Ascending: true,
		Type:      "contributions",
	}
	expecting := SetupMockGetAccountSettlementEarnings(suite.mock, options)
	earnings, err := suite.db.GetAccountSettlementEarnings(
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

func SetupMockGetPending(
	mock sqlmock.Sqlmock,
	accountIDs []string,
) []Votes {
	getRows := sqlmock.NewRows(
		[]string{"channel", "balance"},
	)
	rows := []Votes{}
	for _, channel := range accountIDs {
		balance := decimal.NewFromFloat(
			float64(rand.Intn(100)),
		).Div(
			decimal.NewFromFloat(10),
		)
		rows = append(rows, Votes{channel, balance})
		// append sql result rows
		getRows = getRows.AddRow(
			channel,
			balance,
		)
	}
	mock.ExpectQuery(`
SELECT
	V.channel,
	(.+) as balance
FROM votes V
INNER JOIN surveyor_groups S
ON V.surveyor_id = S.id
WHERE
	V.channel = (.+)
	AND NOT V.transacted
	AND NOT V.excluded
GROUP BY channel`).
		WithArgs(fmt.Sprintf("{%s}", strings.Join(accountIDs, ","))).
		WillReturnRows(getRows)
	return rows
}

func SetupMockGetBalances(
	mock sqlmock.Sqlmock,
	accountIDs []string,
) []Balance {
	getRows := sqlmock.NewRows(
		[]string{"account_id", "account_type", "balance"},
	)
	rows := []Balance{}
	for _, accountID := range accountIDs {
		balance := decimal.NewFromFloat(
			float64(rand.Intn(100)),
		).Div(
			decimal.NewFromFloat(10),
		)
		accountType := uuid.NewV4().String()
		rows = append(rows, Balance{accountID, accountType, balance})
		// append sql result rows
		getRows = getRows.AddRow(
			accountID,
			accountType,
			balance,
		)
	}
	mock.ExpectQuery(`
	SELECT
		account_transactions.account_type as account_type,
		account_transactions.account_id as account_id,
		(.+) as balance
	FROM account_transactions
	WHERE account_id = (.+)
	GROUP BY (.+)`).
		WithArgs(fmt.Sprintf("{%s}", strings.Join(accountIDs, ","))).
		WillReturnRows(getRows)
	return rows
}

func (suite *DatastoreMockTestSuite) TestGetBalances() {
	accountIDs := []string{
		uuid.NewV4().String(),
		uuid.NewV4().String(),
	}

	expectedBalances := SetupMockGetBalances(
		suite.mock,
		accountIDs,
	)
	balances, err := suite.db.GetBalances(
		suite.ctx,
		accountIDs,
	)
	suite.Require().NoError(err)
	suite.Require().Len(*balances, len(accountIDs))

	expectedBalancesMarshalled, err := json.Marshal(expectedBalances)
	suite.Require().NoError(err)
	balancesMarshalled, err := json.Marshal(balances)
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(expectedBalancesMarshalled), string(balancesMarshalled))
}

func (suite *DatastoreMockTestSuite) TestGetPending() {
	accountIDs := []string{
		uuid.NewV4().String(),
		uuid.NewV4().String(),
	}

	expectedVotes := SetupMockGetPending(
		suite.mock,
		accountIDs,
	)
	votes, err := suite.db.GetPending(
		suite.ctx,
		accountIDs,
	)
	suite.Require().NoError(err)
	suite.Require().Len(*votes, len(accountIDs))

	expectedVotesMarshalled, err := json.Marshal(expectedVotes)
	suite.Require().NoError(err)
	balancesMarshalled, err := json.Marshal(votes)
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(expectedVotesMarshalled), string(balancesMarshalled))
}

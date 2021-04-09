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
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	db "github.com/brave-intl/bat-go/utils/datastore"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
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
	options models.AccountEarningsOptions,
) []models.AccountEarnings {
	getRows := sqlmock.NewRows(
		[]string{"channel", "earnings", "account_id"},
	)
	rows := []models.AccountEarnings{}
	for i := 0; i < options.Limit; i++ {
		accountID := fmt.Sprintf("publishers#uuid:%s", uuid.NewV4().String())
		earnings := decimal.NewFromFloat(
			float64(rand.Intn(100)),
		).Div(
			decimal.NewFromFloat(10),
		)
		channel := uuid.NewV4().String()
		rows = append(rows, models.AccountEarnings{
			Channel:   models.Channel(channel),
			Earnings:  earnings,
			AccountID: accountID,
		})
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
	options models.AccountSettlementEarningsOptions,
) []models.AccountSettlementEarnings {
	getRows := sqlmock.NewRows(
		[]string{"channel", "paid", "account_id"},
	)
	rows := []models.AccountSettlementEarnings{}
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
		rows = append(rows, models.AccountSettlementEarnings{
			Channel:   models.Channel(channel),
			Paid:      paid,
			AccountID: accountID,
		})
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

func SetupMockGetPending(
	mock sqlmock.Sqlmock,
	accountIDs []string,
) []models.PendingTransaction {
	getRows := sqlmock.NewRows(
		[]string{"channel", "balance"},
	)
	rows := []models.PendingTransaction{}
	for _, channel := range accountIDs {
		balance := RandomDecimal()
		rows = append(rows, models.PendingTransaction{
			Channel: models.Channel(channel),
			Balance: balance,
		})
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
) []models.Balance {
	getRows := sqlmock.NewRows(
		[]string{"account_id", "account_type", "balance"},
	)
	rows := []models.Balance{}
	for _, accountID := range accountIDs {
		balance := RandomDecimal()
		accountType := uuid.NewV4().String()
		rows = append(rows, models.Balance{
			AccountID: accountID,
			Type:      accountType,
			Balance:   balance,
		})
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

func SetupMockInsertConvertableTransactions(
	mock sqlmock.Sqlmock,
	txs []interface{},
) []models.Transaction {
	rows := []models.Transaction{}
	values := []driver.Value{}
	for _, tx := range txs {
		c := tx.(models.ConvertableTransaction)
		converted := c.ToTxs()
		rows = append(rows, *converted...)
	}
	getRows := sqlmock.NewRows(models.TransactionColumns)
	for _, tx := range rows {
		vals := []driver.Value{
			tx.ID,
			tx.CreatedAt,
			tx.Description,
			tx.TransactionType,
			tx.DocumentID,
			tx.FromAccount,
			tx.FromAccountType,
			tx.ToAccount,
			tx.ToAccountType,
			tx.Amount,
			tx.SettlementCurrency,
			tx.SettlementAmount,
			tx.Channel,
		}
		values = append(values, vals...)
		getRows = getRows.AddRow(vals...)
	}
	query := `
INSERT INTO transactions (.+)
VALUES (.+)
ON CONFLICT DO NOTHING
RETURNING (.+)`
	mock.ExpectQuery(query).
		WithArgs(values...).
		WillReturnRows(getRows)
	return rows
}

func SetupMockGetTransactionsByAccount(
	mock sqlmock.Sqlmock,
	accountID string,
	txTypes ...string,
) []models.Transaction {
	getRows := sqlmock.NewRows(models.TransactionColumns)
	rows := []models.Transaction{}
	args := []driver.Value{accountID}
	var txTypesHash *map[string]bool
	if len(txTypes) > 0 {
		args = append(args, db.JoinStringList(txTypes))
		txTypesHash := &map[string]bool{}
		for _, txType := range txTypes {
			(*txTypesHash)[txType] = true
		}
	}
	channels := CreateIDs(3)
	providerID := uuid.NewV4().String()
	for _, channel := range channels {
		rows = append(rows, ContributeTransaction(channel))
		rows = append(rows, ReferralTransaction(accountID, models.Channel(channel)))
	}
	for i := range channels {
		targetIndex := decimal.NewFromFloat(
			float64(rand.Intn(2)),
		).Mul(decimal.NewFromFloat(float64(i)))
		target := rows[targetIndex.IntPart()]
		rows = append(rows, SettlementTransaction(
			target.ToAccount, // fromAccount
			target.Channel,   // channel
			providerID,       // toAccount
			target.TransactionType,
		))
	}
	for _, tx := range rows {
		if txTypesHash != nil && !(*txTypesHash)[tx.TransactionType] {
			continue
		}
		getRows = getRows.AddRow(
			tx.ID,
			tx.CreatedAt,
			tx.Description,
			tx.TransactionType,
			tx.DocumentID,
			tx.FromAccount,
			tx.FromAccountType,
			tx.ToAccount,
			tx.ToAccountType,
			tx.Amount,
			tx.SettlementCurrency,
			tx.SettlementAmount,
			tx.Channel,
		)
	}
	query := fmt.Sprintf(`
	SELECT %s
	FROM transactions
	WHERE (.+)
	ORDER BY created_at`, strings.Join(models.TransactionColumns, ", "))
	mock.ExpectQuery(query).
		WithArgs(args...).
		WillReturnRows(getRows)
	return rows
}

func SettlementTransaction(
	fromAccount string,
	channel *models.Channel,
	toAccountID string,
	transactionType string,
) models.Transaction {
	transactionType = transactionType + "_settlement"
	provider := "uphold"
	return models.Transaction{
		ID:              uuid.NewV4().String(),
		Channel:         channel,
		CreatedAt:       time.Now(),
		Description:     uuid.NewV4().String(),
		FromAccount:     fromAccount,
		ToAccount:       toAccountID,
		ToAccountType:   provider,
		Amount:          RandomDecimal(),
		TransactionType: transactionType,
	}
}

func ReferralTransaction(accountID string, channel models.Channel) models.Transaction {
	toAccountType := "type"
	return models.Transaction{
		Channel:         &channel,
		CreatedAt:       time.Now(),
		Description:     uuid.NewV4().String(),
		FromAccount:     uuid.NewV4().String(),
		ToAccount:       accountID,
		ToAccountType:   toAccountType,
		Amount:          RandomDecimal(),
		TransactionType: "referral",
	}
}
func ContributeTransaction(toAccount string) models.Transaction {
	toAccountType := "type"
	channel := models.Channel(uuid.NewV4().String())
	return models.Transaction{
		Channel:         &channel,
		CreatedAt:       time.Now(),
		Description:     uuid.NewV4().String(),
		FromAccount:     uuid.NewV4().String(),
		ToAccount:       toAccount,
		ToAccountType:   toAccountType,
		Amount:          RandomDecimal(),
		TransactionType: "contribution",
	}
}

func RandomDecimal() decimal.Decimal {
	return decimal.NewFromFloat(
		float64(rand.Intn(100)),
	).Div(
		decimal.NewFromFloat(10),
	)
}

func CreateIDs(count int) []string {
	list := []string{}
	for i := 0; i < count; i++ {
		list = append(list, uuid.NewV4().String())
	}
	return list
}

func MustMarshal(
	assertions *require.Assertions,
	structure interface{},
) string {
	marshalled, err := json.Marshal(structure)
	assertions.NoError(err)
	return string(marshalled)
}

func (suite *DatastoreMockTestSuite) TestGetAccountEarnings() {
	options := models.AccountEarningsOptions{
		Limit:     5,
		Ascending: true,
		Type:      "contributions",
	}
	expected := SetupMockGetAccountEarnings(suite.mock, options)
	actual := suite.GetAccountEarnings(
		options,
	)

	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expected),
		MustMarshal(suite.Require(), actual),
	)
}

func (suite *DatastoreMockTestSuite) GetAccountEarnings(
	options models.AccountEarningsOptions,
) *[]models.AccountEarnings {
	earnings, err := suite.db.GetAccountEarnings(
		suite.ctx,
		options,
	)
	suite.Require().NoError(err)
	suite.Require().Len(*earnings, options.Limit)
	return earnings
}
func (suite *DatastoreMockTestSuite) TestGetAccountSettlementEarnings() {
	options := models.AccountSettlementEarningsOptions{
		Limit:     5,
		Ascending: true,
		Type:      "contributions",
	}
	expectSettlementEarnings := SetupMockGetAccountSettlementEarnings(suite.mock, options)
	actualSettlementEarnings := suite.GetAccountSettlementEarnings(options)
	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expectSettlementEarnings),
		MustMarshal(suite.Require(), actualSettlementEarnings),
	)
}

func (suite *DatastoreMockTestSuite) GetAccountSettlementEarnings(
	options models.AccountSettlementEarningsOptions,
) *[]models.AccountSettlementEarnings {
	earnings, err := suite.db.GetAccountSettlementEarnings(
		suite.ctx,
		options,
	)
	suite.Require().NoError(err)
	suite.Require().Len(*earnings, options.Limit)
	return earnings
}

func (suite *DatastoreMockTestSuite) TestGetBalances() {
	accountIDs := CreateIDs(3)

	expectedBalances := SetupMockGetBalances(
		suite.mock,
		accountIDs,
	)
	actualBalances := suite.GetBalances(accountIDs)
	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expectedBalances),
		MustMarshal(suite.Require(), actualBalances),
	)
}

func (suite *DatastoreMockTestSuite) GetBalances(accountIDs []string) *[]models.Balance {
	balances, err := suite.db.GetBalances(
		suite.ctx,
		accountIDs,
	)
	suite.Require().NoError(err)
	suite.Require().Len(*balances, len(accountIDs))
	return balances
}

func (suite *DatastoreMockTestSuite) TestGetPending() {
	accountIDs := CreateIDs(3)

	expectedVotes := SetupMockGetPending(
		suite.mock,
		accountIDs,
	)
	actualVotes := suite.GetPending(accountIDs)
	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expectedVotes),
		MustMarshal(suite.Require(), actualVotes),
	)
}

func (suite *DatastoreMockTestSuite) GetPending(accountIDs []string) *[]models.PendingTransaction {
	votes, err := suite.db.GetPending(
		suite.ctx,
		accountIDs,
	)
	suite.Require().NoError(err)
	suite.Require().Len(*votes, len(accountIDs))
	return votes
}

func (suite *DatastoreMockTestSuite) TestGetTransactionsByAccount() {
	accountID := CreateIDs(1)[0]

	expectedTransactions := SetupMockGetTransactionsByAccount(
		suite.mock,
		accountID,
	)
	actualTransaction := suite.GetTransactionsByAccount(
		len(expectedTransactions),
		accountID,
		nil,
	)
	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expectedTransactions),
		MustMarshal(suite.Require(), actualTransaction),
	)
}

func (suite *DatastoreMockTestSuite) GetTransactionsByAccount(
	count int,
	accountID string,
	txTypes []string,
) *[]models.Transaction {
	transactions, err := suite.db.GetTransactionsByAccount(
		suite.ctx,
		accountID,
		txTypes,
	)
	suite.Require().NoError(err)
	suite.Require().Len(*transactions, count)
	return transactions
}

func (suite *DatastoreMockTestSuite) TestInsertSettlement() {
	now := time.Now()
	settlement := &models.Settlement{
		AltCurrency:    altcurrency.BAT,
		Probi:          altcurrency.BAT.ToProbi(decimal.NewFromFloat(4.75)),
		Fees:           altcurrency.BAT.ToProbi(decimal.NewFromFloat(0.25)),
		Amount:         decimal.NewFromFloat(4),
		Currency:       "USD",
		Owner:          fmt.Sprintf("publishers#uuid:%s", uuid.NewV4().String()),
		Channel:        models.Channel("brave.com"),
		Type:           "contribution",
		Hash:           uuid.NewV4().String(),
		SettlementID:   uuid.NewV4().String(),
		DocumentID:     uuid.NewV4().String(),
		Address:        uuid.NewV4().String(),
		ExecutedAt:     &now,
		WalletProvider: nil,
	}
	settlements := []interface{}{settlement}
	expect := SetupMockInsertConvertableTransactions(
		suite.mock,
		settlements,
	)
	actual := suite.InsertConvertableTransactions(settlements)

	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expect),
		MustMarshal(suite.Require(), actual),
	)
}

func (suite *DatastoreMockTestSuite) InsertConvertableTransactions(txs []interface{}) *[]models.Transaction {
	inserted, err := suite.db.InsertConvertableTransactions(suite.ctx, &txs)
	suite.Require().NoError(err)
	return inserted
}

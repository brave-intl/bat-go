// +build test

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
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type DatastoreMockSuite struct {
	suite.Suite
	ctx  context.Context
	db   Datastore
	mock sqlmock.Sqlmock
}

func TestDatastoreMockSuite(t *testing.T) {
	suite.Run(t, new(DatastoreMockSuite))
	// if os.Getenv("EYESHADE_DB_URL") != "" {
	// 	suite.Run(t, new(DatastoreSuite))
	// }
}

func (suite *DatastoreMockSuite) SetupSuite() {
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
SELECT
	channel,
	(.+) AS earnings,
	account_id
FROM account_transactions
WHERE account_type = 'owner'
	AND transaction_type = (.+)
GROUP BY (.+)
ORDER BY earnings (.+)
LIMIT (.+)`).
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
		args = append(args, targetTime.Format(time.RFC3339))
	}
	untilDate := options.UntilDate
	if untilDate == nil {
		untilDatePrep := targetTime.Add(time.Hour * 24 * time.Duration(options.Limit))
		untilDate = &untilDatePrep
	} else {
		args = append(args, untilDate.Format(time.RFC3339))
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
SELECT
	channel,
	(.+) AS paid,
	account_id
FROM account_transactions
WHERE (.+)
GROUP BY (.+)
ORDER BY paid (.+)
LIMIT (.+)`).
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
	(.+) AS balance
FROM votes V
INNER JOIN surveyor_groups S
ON V.surveyor_id = S.id
WHERE
	V.channel = (.+)
	AND NOT V.transacted
	AND NOT V.excluded
GROUP BY channel`).
		WithArgs(pq.Array(accountIDs)).
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
		account_transactions.account_type AS account_type,
		account_transactions.account_id AS account_id,
		(.+) AS balance
	FROM account_transactions
	WHERE account_id = (.+)
	GROUP BY (.+)`).
		WithArgs(pq.Array(accountIDs)).
		WillReturnRows(getRows)
	return rows
}

func collectTxValues(
	txs []models.ConvertableTransaction,
) (
	[]models.Transaction,
	*sqlmock.Rows,
	[]driver.Value,
) {
	rows := []models.Transaction{}
	values := []driver.Value{}
	for _, tx := range txs {
		rows = append(rows, tx.ToTxs()...)
	}
	mockRows := sqlmock.NewRows(models.TransactionColumns)
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
		mockRows = mockRows.AddRow(vals...)
		vals[1] = sqlmock.AnyArg()
		values = append(values, vals...)
	}
	return rows, mockRows, values
}

func SetupMockInsertConvertableTransactions(
	mock sqlmock.Sqlmock,
	roMock sqlmock.Sqlmock,
	txs []models.ConvertableTransaction,
) []models.Transaction {
	rows, mockRows, values := collectTxValues(txs)
	mock.ExpectExec(`
INSERT INTO transactions (.+)
VALUES (.+)
ON CONFLICT DO NOTHING`).
		WithArgs(values...).
		WillReturnResult(sqlmock.NewResult(2, 3))
	roMock.ExpectQuery(`SELECT (.+)
FROM transactions`).
		WillReturnRows(mockRows)
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
		args = append(args, pq.Array(txTypes))
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
	return models.Transaction{
		ID:              uuid.NewV4().String(),
		Channel:         channel,
		CreatedAt:       time.Now(),
		Description:     uuid.NewV4().String(),
		FromAccount:     fromAccount,
		ToAccount:       toAccountID,
		ToAccountType:   models.Providers.Uphold,
		Amount:          RandomDecimal(),
		TransactionType: models.SettlementKeys.AddSuffix(transactionType),
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
		TransactionType: models.TransactionTypes.Referral,
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
		TransactionType: models.TransactionTypes.Contribution,
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

func (suite *DatastoreMockSuite) TestGetAccountEarnings() {
	options := models.AccountEarningsOptions{
		Limit:     5,
		Ascending: true,
		Type:      models.TransactionTypes.Contribution,
	}
	expect := SetupMockGetAccountEarnings(suite.mock, options)
	actual := suite.GetAccountEarnings(
		options,
	)

	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expect),
		MustMarshal(suite.Require(), actual),
	)
}

func (suite *DatastoreMockSuite) GetAccountEarnings(
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
func (suite *DatastoreMockSuite) TestGetAccountSettlementEarnings() {
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

func (suite *DatastoreMockSuite) GetAccountSettlementEarnings(
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

func (suite *DatastoreMockSuite) TestGetBalances() {
	accountIDs := CreateIDs(3)

	expect := SetupMockGetBalances(
		suite.mock,
		accountIDs,
	)
	actual := suite.GetBalances(accountIDs)
	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expect),
		MustMarshal(suite.Require(), actual),
	)
}

func (suite *DatastoreMockSuite) GetBalances(accountIDs []string) *[]models.Balance {
	balances, err := suite.db.GetBalances(
		suite.ctx,
		accountIDs,
	)
	suite.Require().NoError(err)
	suite.Require().Len(*balances, len(accountIDs))
	return balances
}

func (suite *DatastoreMockSuite) TestGetPending() {
	accountIDs := CreateIDs(3)

	expect := SetupMockGetPending(
		suite.mock,
		accountIDs,
	)
	actual := suite.GetPending(accountIDs)
	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expect),
		MustMarshal(suite.Require(), actual),
	)
}

func (suite *DatastoreMockSuite) GetPending(accountIDs []string) *[]models.PendingTransaction {
	votes, err := suite.db.GetPending(
		suite.ctx,
		accountIDs,
	)
	suite.Require().NoError(err)
	suite.Require().Len(*votes, len(accountIDs))
	return votes
}

func (suite *DatastoreMockSuite) TestGetTransactionsByAccount() {
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

func (suite *DatastoreMockSuite) GetTransactionsByAccount(
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

func (suite *DatastoreMockSuite) TestInsertSettlement() {
	settlements := CreateSettlements(1, models.TransactionTypes.Contribution)
	convertableTransactions := models.SettlementsToConvertableTransactions(settlements...)
	expect := SetupMockInsertConvertableTransactions(
		suite.mock,
		suite.mock,
		convertableTransactions,
	)
	actual := suite.InsertConvertableTransactions(convertableTransactions)

	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expect),
		MustMarshal(suite.Require(), actual),
	)
}

func (suite *DatastoreMockSuite) InsertConvertableTransactions(
	txs []models.ConvertableTransaction,
) *[]models.Transaction {
	err := suite.db.InsertConvertableTransactions(suite.ctx, txs)
	suite.Require().NoError(err)
	transactions, err := suite.db.GetTransactions(suite.ctx)
	suite.Require().NoError(err)
	return transactions
}

func CreateSettlements(count int, txType string) []models.Settlement {
	settlements := []models.Settlement{}
	for i := 0; i < count; i++ {
		bat := decimal.NewFromFloat(5)
		fees := bat.Mul(decimal.NewFromFloat(0.05))
		batSubFees := bat.Sub(fees)
		settlements = append(settlements, models.Settlement{
			AltCurrency:  altcurrency.BAT,
			Probi:        altcurrency.BAT.ToProbi(batSubFees),
			Fees:         altcurrency.BAT.ToProbi(fees),
			Fee:          decimal.Zero,
			Commission:   decimal.Zero,
			Amount:       bat,
			Currency:     altcurrency.BAT.String(),
			Owner:        fmt.Sprintf("publishers#uuid:%s", uuid.NewV4().String()),
			Channel:      models.Channel("brave.com"),
			Hash:         uuid.NewV4().String(),
			Type:         txType,
			SettlementID: uuid.NewV4().String(),
			DocumentID:   uuid.NewV4().String(),
			Address:      uuid.NewV4().String(),
		})
	}
	return settlements
}

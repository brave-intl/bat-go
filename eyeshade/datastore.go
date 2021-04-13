package eyeshade

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/eyeshade/countries"
	"github.com/brave-intl/bat-go/eyeshade/models"
	appctx "github.com/brave-intl/bat-go/utils/context"
	db "github.com/brave-intl/bat-go/utils/datastore"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/getsentry/sentry-go"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

var (
	// ErrLimitRequired signifies that the query requires a limit
	ErrLimitRequired = errors.New("query requires a limit")
	// ErrLimitReached signifies that the limit has been exceeded
	ErrLimitReached = errors.New("query limit reached")
	// ErrNeedTimeConstraints needs both a start and an end time
	ErrNeedTimeConstraints = errors.New("need both start and end date")
)

// Datastore holds methods for interacting with database
type Datastore interface {
	grantserver.Datastore
	ResolveConnection(ctx context.Context) (context.Context, *sqlx.Tx, error)
	WithTx(ctx context.Context) (context.Context, *sqlx.Tx, error)
	Rollback(ctx context.Context)
	Commit(ctx context.Context) error
	GetAccountEarnings(
		ctx context.Context,
		options models.AccountEarningsOptions,
	) (*[]models.AccountEarnings, error)
	GetAccountSettlementEarnings(
		ctx context.Context,
		options models.AccountSettlementEarningsOptions,
	) (*[]models.AccountSettlementEarnings, error)
	GetBalances(
		ctx context.Context,
		accountIDs []string,
	) (*[]models.Balance, error)
	GetPending(
		ctx context.Context,
		accountIDs []string,
	) (*[]models.PendingTransaction, error)
	GetTransactions(
		ctx context.Context,
		constraints ...map[string]string,
	) (*[]models.Transaction, error)
	GetTransactionsByAccount(
		ctx context.Context,
		accountID string,
		txTypes []string,
	) (*[]models.Transaction, error)
	InsertConvertableTransactions(
		ctx context.Context,
		txs []models.ConvertableTransaction,
	) error
	InsertTransactions(
		ctx context.Context,
		txs *[]models.Transaction,
	) error
	GetActiveCountryGroups(ctx context.Context) (*[]countries.Group, error)
	GetReferralGroups(
		ctx context.Context,
		activeAt inputs.Time,
	) (*[]countries.ReferralGroup, error)
	GetSettlementStats(
		ctx context.Context,
		options models.SettlementStatOptions,
	) (*models.SettlementStat, error)
	GetGrantStats(
		ctx context.Context,
		options models.GrantStatOptions,
	) (*models.GrantStat, error)
	InsertSuggestions(
		ctx context.Context,
		suggestions []models.Suggestion,
	) error
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	grantserver.Postgres
}

// NewFromConnection creates new datastores from connection objects
func NewFromConnection(pg *grantserver.Postgres, name string) Datastore {
	return &DatastoreWithPrometheus{
		base:         &Postgres{*pg},
		instanceName: name,
	}
}

// NewDB creates a new Postgres Datastore
func NewDB(
	databaseURL string,
	performMigration bool,
	name string,
	dbStatsPrefix ...string,
) (Datastore, error) {
	pg, err := grantserver.NewPostgres(
		databaseURL,
		performMigration,
		"eyeshade", // to follow the migration track for eyeshade
		dbStatsPrefix...,
	)
	if err != nil {
		return nil, err
	}
	return NewFromConnection(pg, name), nil
}

// NewConnections creates postgres connections
func NewConnections() (Datastore, Datastore, error) {
	var eyeshadeRoPg Datastore
	eyeshadePg, err := NewDB(
		os.Getenv("EYESHADE_DB_URL"),
		true,
		"eyeshade_datastore",
		"eyeshade_db",
	)
	if err != nil {
		sentry.CaptureException(err)
		log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
	}

	roDB := os.Getenv("EYESHADE_DB_RO_URL")
	if len(roDB) > 0 {
		eyeshadeRoPg, err = NewDB(
			roDB,
			false,
			"eyeshade_ro_datastore",
			"eyeshade_read_only_db",
		)
		if err != nil {
			sentry.CaptureException(err)
			log.Error().Err(err).Msg("Could not start reader postgres connection")
		}
	}
	return eyeshadePg, eyeshadeRoPg, err
}

// WithTx manages transaction to context attachment
func (pg *Postgres) WithTx(ctx context.Context) (context.Context, *sqlx.Tx, error) {
	tx, err := pg.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return ctx, tx, err
	}
	return context.WithValue(ctx, appctx.TxCTXKey, tx), tx, nil
}

// Rollback rolls back a transaction if on the right level
func (pg *Postgres) Rollback(ctx context.Context) {
	rollback, ok := ctx.Value(appctx.TxRollbackCTXKey).(bool)
	if ok && !rollback {
		return // this is a nested rollback call
	}
	tx, ok := ctx.Value(appctx.TxCTXKey).(*sqlx.Tx)
	if !ok {
		return // tx on context does not exist
	}
	err := tx.Rollback()
	if err == nil || err == sql.ErrTxDone {
		return // rollback or commit was already called
	}
	fmt.Println(err)
}

// ResolveConnection creates a transaction or uses the in progress one
func (pg *Postgres) ResolveConnection(ctx context.Context) (context.Context, *sqlx.Tx, error) {
	tx, ok := ctx.Value(appctx.TxCTXKey).(*sqlx.Tx)
	if ok {
		return context.WithValue(ctx, appctx.TxRollbackCTXKey, false), tx, nil
	}
	ctx, tx, err := pg.WithTx(ctx)
	return context.WithValue(ctx, appctx.TxRollbackCTXKey, true), tx, err
}

// Commit commits a transaction on the context if on the right level
func (pg *Postgres) Commit(ctx context.Context) error {
	rollback, ok := ctx.Value(appctx.TxRollbackCTXKey).(bool)
	if !ok || !rollback {
		return nil // not the right context value or not the right tx level
	}
	tx, ok := ctx.Value(appctx.TxRollbackCTXKey).(*sqlx.Tx)
	if !ok {
		return errors.New("unable to find tx")
	}
	return tx.Commit()
}

// GetAccountEarnings gets the account earnings for a subset of ids
func (pg *Postgres) GetAccountEarnings(
	ctx context.Context,
	options models.AccountEarningsOptions,
) (*[]models.AccountEarnings, error) {
	order := "desc"
	if options.Ascending {
		order = "asc"
	}
	limit := options.Limit
	if limit < 1 {
		return nil, ErrLimitRequired
	} else if limit > 1000 {
		return nil, ErrLimitReached
	}
	// remove the `s`
	txType := options.Type[:len(options.Type)-1]
	statement := fmt.Sprintf(`
select
	channel,
	coalesce(sum(amount), 0.0) as earnings,
	account_id
from account_transactions
where account_type = 'owner'
	and transaction_type = $1
group by (account_id, channel)
order by earnings %s
limit $2`, order)
	earnings := []models.AccountEarnings{}
	err := pg.RawDB().SelectContext(
		ctx,
		&earnings,
		statement,
		txType,
		limit,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return &earnings, nil
}

// GetAccountSettlementEarnings gets the account settlement earnings for a subset of ids
func (pg *Postgres) GetAccountSettlementEarnings(
	ctx context.Context,
	options models.AccountSettlementEarningsOptions,
) (*[]models.AccountSettlementEarnings, error) {
	order := "desc"
	if options.Ascending {
		order = "asc"
	}
	limit := options.Limit
	if limit < 1 {
		return nil, ErrLimitRequired
	} else if limit > 1000 {
		return nil, ErrLimitReached
	}
	txType := options.Type[:len(options.Type)-1]
	txType = fmt.Sprintf(`%s_settlement`, txType)
	timeConstraints := ""
	constraints := []interface{}{
		txType,
		limit,
	}
	if options.UntilDate != nil {
		// if end date exists, start date must also
		if options.StartDate == nil {
			return nil, ErrNeedTimeConstraints
		}
		constraints = append(
			constraints,
			options.StartDate.Format(time.RFC3339),
			options.UntilDate.Format(time.RFC3339),
		)
		timeConstraints = `
and created_at >= $3
and created_at < $4`
	}
	statement := fmt.Sprintf(`
select
	channel,
	coalesce(sum(-amount), 0.0) as paid,
	account_id
from account_transactions
where
		account_type = 'owner'
and transaction_type = $1%s
group by (account_id, channel)
order by paid %s
limit $2`, timeConstraints, order)
	earnings := []models.AccountSettlementEarnings{}

	err := pg.RawDB().SelectContext(
		ctx,
		&earnings,
		statement,
		constraints...,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return &earnings, nil
}

// GetBalances gets the account settlement earnings for a subset of ids
func (pg *Postgres) GetBalances(
	ctx context.Context,
	accountIDs []string,
) (*[]models.Balance, error) {
	statement := `
	SELECT
		account_transactions.account_type as account_type,
		account_transactions.account_id as account_id,
		COALESCE(SUM(account_transactions.amount), 0.0) as balance
	FROM account_transactions
	WHERE account_id = any($1::text[])
	GROUP BY (account_transactions.account_id, account_transactions.account_type)`
	balances := []models.Balance{}

	err := pg.RawDB().SelectContext(
		ctx,
		&balances,
		statement,
		db.JoinStringList(accountIDs),
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return &balances, nil
}

// GetPending retrieves the pending votes tied to an account id
func (pg *Postgres) GetPending(
	ctx context.Context,
	accountIDs []string,
) (*[]models.PendingTransaction, error) {
	statement := `
SELECT
	V.channel,
	SUM(V.tally * S.price)::TEXT as balance
FROM votes V
INNER JOIN surveyor_groups S
ON V.surveyor_id = S.id
WHERE
	V.channel = any($1::text[])
	AND NOT V.transacted
	AND NOT V.excluded
GROUP BY channel`
	votes := []models.PendingTransaction{}

	err := pg.RawDB().SelectContext(
		ctx,
		&votes,
		statement,
		db.JoinStringList(accountIDs),
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return &votes, nil
}

// GetTransactions gets transactions
func (pg *Postgres) GetTransactions(
	ctx context.Context,
	constraints ...map[string]string,
) (*[]models.Transaction, error) {
	statement := fmt.Sprintf(`
SELECT %s
FROM transactions`,
		strings.Join(models.TransactionColumns, ", "),
	)
	transactions := []models.Transaction{}
	return &transactions, pg.RawDB().SelectContext(ctx, &transactions, statement)
}

// GetTransactionsByAccount retrieves the transactions tied to an account id
func (pg *Postgres) GetTransactionsByAccount(
	ctx context.Context,
	accountID string,
	txTypes []string,
) (*[]models.Transaction, error) {
	typeExtension := ""
	args := []interface{}{accountID}
	if len(txTypes) > 0 {
		args = append(args, db.JoinStringList(txTypes))
		typeExtension = "AND transaction_type = ANY($2::text[])"
	}
	statement := fmt.Sprintf(`
SELECT %s
FROM transactions
WHERE (
  from_account = $1
  OR to_account = $1
) %s
ORDER BY created_at`,
		strings.Join(models.TransactionColumns, ", "),
		typeExtension,
	)
	transactions := []models.Transaction{}

	err := pg.RawDB().SelectContext(
		ctx,
		&transactions,
		statement,
		args...,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return &transactions, nil
}

// InsertConvertableTransactions inserts a list of settlement transactions all at the same time
func (pg *Postgres) InsertConvertableTransactions(ctx context.Context, targets []models.ConvertableTransaction) error {
	txs, err := convertToTxs(targets)
	if err != nil {
		return err
	}
	return pg.InsertTransactions(ctx, txs)
}

func convertToTxs(convertables []models.ConvertableTransaction) (*[]models.Transaction, error) {
	txs := []models.Transaction{}
	for _, con := range convertables {
		if con.Ignore() {
			continue
		}
		if err := con.Valid(); err != nil {
			return nil, &errorutils.MultiError{
				Errs: []error{
					models.ErrConvertableFailedValidation,
					err,
				},
			}
		}
		txs = append(txs, con.ToTxs()...)
	}
	return &txs, nil
}

// InsertTransactions is a generalizable transaction insertion function
func (pg *Postgres) InsertTransactions(
	ctx context.Context,
	txs *[]models.Transaction,
) error {
	statement := fmt.Sprintf(`
INSERT INTO transactions ( %s )
VALUES ( %s )
ON CONFLICT DO NOTHING`,
		strings.Join(models.TransactionColumns, ", "),
		strings.Join(db.ColumnsToParamNames(models.TransactionColumns), ", "),
	)
	_, err := pg.RawDB().NamedExecContext(
		ctx,
		statement,
		*txs,
	)
	return err
}

// GetSettlementStats gets stats about settlements
func (pg *Postgres) GetSettlementStats(ctx context.Context, options models.SettlementStatOptions) (*models.SettlementStat, error) {
	args := []interface{}{
		options.Type,
		options.Start,
		options.Until,
	}
	extra := ""
	if options.Currency != nil {
		args = append(args, options.Currency)
		extra = "AND settlement_currency = $4"
	}
	statement := fmt.Sprintf(`
SELECT
	sum(amount) as amount
FROM transactions
WHERE
	transaction_type = $1 %s
AND created_at >= to_timestamp($2)
AND created_at < to_timestamp($3)`,
		extra,
	)
	var stats models.SettlementStat
	return &stats, pg.GetContext(ctx, &stats, statement, args...)
}

// GetGrantStats gets stats about grants
func (pg *Postgres) GetGrantStats(ctx context.Context, options models.GrantStatOptions) (*models.GrantStat, error) {
	statement := `
SELECT
	count(*) as count,
	sum(amount) as amount
FROM votes
WHERE
		cohort = $1::text
AND created_at >= to_timestamp($2)
AND created_at < to_timestamp($3)`
	var stats models.GrantStat
	return &stats, pg.GetContext(
		ctx,
		&stats,
		statement,
		options.Type,
		options.Start,
		options.Until,
	)
}

// GetActiveCountryGroups gets country groups to match their amount with their designated payout
func (pg *Postgres) GetActiveCountryGroups(ctx context.Context) (*[]countries.Group, error) {
	statement := `
SELECT
	id,
	amount,
	currency,
	active_at AS "activeAt"
FROM geo_referral_groups
WHERE
	active_at <= CURRENT_TIMESTAMP
ORDER BY active_at DESC`
	groups := []countries.Group{}
	return &groups, pg.RawDB().SelectContext(ctx, &groups, statement)
}

// GetReferralGroups gets referral groups active by a certain time
func (pg *Postgres) GetReferralGroups(ctx context.Context, activeAt inputs.Time) (*[]countries.ReferralGroup, error) {
	statement := `
SELECT
  id,
  active_at,
  name,
  amount,
  currency,
  countries.codes AS codes
FROM geo_referral_groups, (
  SELECT
    group_id,
    array_agg(country_code) AS codes
  FROM geo_referral_countries
  GROUP BY group_id
) AS countries
WHERE
    geo_referral_groups.active_at <= $1
AND countries.group_id = geo_referral_groups.id`
	groups := []countries.ReferralGroup{}
	err := pg.SelectContext(ctx, &groups, statement, activeAt)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return &groups, nil
}

// InsertSuggestions inserts a series of suggstions into the votes table
func (pg *Postgres) InsertSuggestions(
	ctx context.Context,
	suggestions []models.Suggestion,
) error {
	statementInsertSurveyor := `
insert into surveyor_groups (id, price, virtual) values ($1, $2, true)
on conflict (id) do nothing`
	statementVotesUpdate := `
insert into votes (id, cohort, tally, excluded, channel, surveyor_id) values ($1, $2, $3, $4, $5, $6)
on conflict (id) do update set updated_at = current_timestamp, tally = votes.tally + $3`
	date := time.Now().Format("2021-01-01")
	voteValue, err := decimal.NewFromString("0.25")
	if err != nil {
		return err
	}
	promotionToSurveyor := map[string]string{}
	type Query struct {
		statement string
		args      []interface{}
	}
	queries := []Query{}
	for _, suggestion := range suggestions {
		for _, funding := range suggestion.Funding {
			surveyorID := promotionToSurveyor[funding.Promotion]
			if surveyorID != "" {
				continue
			}
			surveyorID = fmt.Sprintf("%s_%s", date, funding.Promotion)
			promotionToSurveyor[funding.Promotion] = surveyorID
			queries = append(queries, Query{
				statement: statementInsertSurveyor,
				args: []interface{}{
					surveyorID,
					voteValue.String(),
				},
			})
		}
	}
	for _, suggestion := range suggestions {
		for _, funding := range suggestion.Funding {
			surveyorID := promotionToSurveyor[funding.Promotion]
			tally := funding.Amount.Div(voteValue)
			queries = append(queries, Query{
				statement: statementVotesUpdate,
				args: []interface{}{
					suggestion.GenerateID(funding.Cohort, surveyorID),
					funding.Cohort,
					tally.String(),
					false,
					suggestion.Channel.Normalize().String(),
					surveyorID,
				},
			})
		}
	}
	ctx, tx, err := pg.ResolveConnection(ctx)
	if err != nil {
		return err
	}
	defer pg.Rollback(ctx)
	for _, q := range queries {
		_, err := tx.ExecContext(ctx, q.statement, q.args...)
		if err != nil {
			return err
		}
	}
	return pg.Commit(ctx)
}

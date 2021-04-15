package eyeshade

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/eyeshade/countries"
	"github.com/brave-intl/bat-go/eyeshade/models"
	datastoreutils "github.com/brave-intl/bat-go/utils/datastore"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/getsentry/sentry-go"
	"github.com/lib/pq"
	"github.com/rs/zerolog/log"
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
	InsertVotes(ctx context.Context, votes []models.Vote) error
	InsertSurveyors(ctx context.Context, surveyors *[]models.Surveyor) error
	InsertBallots(ctx context.Context, ballots *[]models.Ballot) error
	SeedDB(context.Context) error
	GetBallotsByID(context.Context, ...string) (*[]models.Ballot, error)
	GetTransactionsByID(
		ctx context.Context,
		documentIDs ...string,
	) (*[]models.Transaction, error)
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
	WHERE account_id = any($1::TEXT[])
	GROUP BY (account_transactions.account_id, account_transactions.account_type)`
	balances := []models.Balance{}

	err := pg.RawDB().SelectContext(
		ctx,
		&balances,
		statement,
		pq.Array(accountIDs),
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
	V.channel = any($1::TEXT[])
	AND NOT V.transacted
	AND NOT V.excluded
GROUP BY channel`
	votes := []models.PendingTransaction{}

	err := pg.RawDB().SelectContext(
		ctx,
		&votes,
		statement,
		pq.Array(accountIDs),
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

// GetTransactionsByID retrieves transactions by a document id
func (pg *Postgres) GetTransactionsByID(
	ctx context.Context,
	ids ...string,
) (*[]models.Transaction, error) {
	statement := fmt.Sprintf(`
SELECT %s
FROM transactions
WHERE id = any($1::UUID[])
ORDER BY id ASC`,
		strings.Join(models.TransactionColumns, ", "),
	)
	transactions := []models.Transaction{}
	return &transactions, pg.RawDB().SelectContext(ctx, &transactions, statement, pq.Array(ids))
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
		args = append(args, pq.Array(txTypes))
		typeExtension = "AND transaction_type = ANY($2::TEXT[])"
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
		strings.Join(datastoreutils.ColumnsToParamNames(models.TransactionColumns), ", "),
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
		cohort = $1::TEXT
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
	active_at
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

// GetBallotsByID gets vote rows by id
func (pg *Postgres) GetBallotsByID(ctx context.Context, ids ...string) (*[]models.Ballot, error) {
	statement := fmt.Sprintf(`
SELECT %s
FROM votes
WHERE id = any($1::UUID[])
ORDER BY (id) asc`,
		strings.Join(models.BallotColumns, ", "),
	)
	ballots := []models.Ballot{}
	return &ballots, pg.RawDB().SelectContext(ctx, &ballots, statement, pq.Array(ids))
}

// InsertVotes inserts votes into the votes table after inserting surveyors
func (pg *Postgres) InsertVotes(ctx context.Context, votes []models.Vote) error {
	ctx, _, err := pg.ResolveConnection(ctx)
	if err != nil {
		return err
	}
	defer pg.Rollback(ctx)
	surveyors, ballots, err := pg.convertToBallots(ctx, votes)
	if err != nil {
		return err
	}
	if err := pg.InsertSurveyors(ctx, surveyors); err != nil {
		return err
	}
	ballots = models.CondenseBallots(ballots)
	if err := pg.InsertBallots(ctx, ballots); err != nil {
		return err
	}
	return pg.Commit(ctx)
}

// InsertSurveyors inserts surveyors to the surveyors_groups table
func (pg *Postgres) InsertSurveyors(
	ctx context.Context,
	surveyors *[]models.Surveyor,
) error {
	ctx, tx, err := pg.ResolveConnection(ctx)
	if err != nil {
		return err
	}
	defer pg.Rollback(ctx)
	columns := []string{"id", "price", "virtual"}
	statement := fmt.Sprintf(`
INSERT INTO surveyor_groups ( %s )
VALUES ( %s )
ON CONFLICT ( id ) DO NOTHING`,
		strings.Join(columns, ", "),
		strings.Join(datastoreutils.ColumnsToParamNames(columns), ", "),
	)
	_, err = tx.NamedExecContext(
		ctx,
		statement,
		*surveyors,
	)
	if err != nil {
		return err
	}
	return pg.Commit(ctx)
}

// InsertBallots inserts ballots to the votes table
func (pg *Postgres) InsertBallots(ctx context.Context, ballots *[]models.Ballot) error {
	ctx, tx, err := pg.ResolveConnection(ctx)
	if err != nil {
		return err
	}
	defer pg.Rollback(ctx)
	statement := fmt.Sprintf(`INSERT INTO votes ( %s )
VALUES ( %s )
ON CONFLICT ( id ) DO UPDATE
SET updated_at = CURRENT_TIMESTAMP,
tally = votes.tally + EXCLUDED.tally
WHERE votes.id = EXCLUDED.id`,
		strings.Join(models.BallotColumns, ", "),
		strings.Join(datastoreutils.ColumnsToParamNames(models.BallotColumns), ", "),
	)
	_, err = tx.NamedExecContext(
		ctx,
		statement,
		*ballots,
	)
	if err != nil {
		return fmt.Errorf("multi votes insert failed %v", err)
	}
	return pg.Commit(ctx)
}

// GetSurveyorsByID gets surveyors by their id
func (pg *Postgres) GetSurveyorsByID(ctx context.Context, ids []string) (*[]models.Surveyor, error) {
	surveyors := []models.Surveyor{}
	ctx, tx, err := pg.ResolveConnection(ctx)
	if err != nil {
		return nil, err
	}
	defer pg.Rollback(ctx)
	statement := fmt.Sprintf(`SELECT %s
FROM surveyor_groups
WHERE id = ANY($1::TEXT[])`, strings.Join(models.SurveyorColumns, ", "))
	err = tx.SelectContext(ctx, &surveyors, statement, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	return &surveyors, pg.Commit(ctx)
}

func (pg *Postgres) convertToBallots(
	ctx context.Context,
	votes []models.Vote,
) (*[]models.Surveyor, *[]models.Ballot, error) {
	date := time.Now().Format("2021-01-01")
	_, surveyorIDs := models.CollectSurveyorIDs(date, votes)
	insertedSurveyors, err := pg.GetSurveyorsByID(ctx, surveyorIDs)
	if err != nil {
		return nil, nil, err
	}
	frozenSurveyors := models.SurveyorIDsToFrozen(*insertedSurveyors)
	surveyors, ballots := models.CollectBallots(date, votes, frozenSurveyors)
	return &surveyors, &ballots, nil
}

// SeedDB seeds the db with the appropriate seed files
func (pg *Postgres) SeedDB(ctx context.Context) error {
	dir := "./seeds/"
	fileInfos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() {
			continue
		}
		input := path.Join(dir, fileInfo.Name())
		statement, err := ioutil.ReadFile(input)
		if err != nil {
			return err
		}
		_, err = pg.RawDB().ExecContext(ctx, string(statement))
		if err != nil {
			return err
		}
	}
	return nil
}

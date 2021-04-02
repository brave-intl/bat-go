package eyeshade

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	db "github.com/brave-intl/bat-go/utils/datastore"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/getsentry/sentry-go"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

var (
	// ErrLimitRequired signifies that the query requires a limit
	ErrLimitRequired = errors.New("query requires a limit")
	// ErrLimitReached signifies that the limit has been exceeded
	ErrLimitReached = errors.New("query limit reached")
	// ErrNeedTimeConstraints needs both a start and an end time
	ErrNeedTimeConstraints = errors.New("need both start and end date")

	settlementTypes = map[string]bool{
		"contribution_settlement": true,
		"referral_settlement":     true,
	}
)

// Datastore holds methods for interacting with database
type Datastore interface {
	grantserver.Datastore
	GetAccountEarnings(
		ctx context.Context,
		options AccountEarningsOptions,
	) (*[]AccountEarnings, error)
	GetAccountSettlementEarnings(
		ctx context.Context,
		options AccountSettlementEarningsOptions,
	) (*[]AccountSettlementEarnings, error)
	GetBalances(
		ctx context.Context,
		accountIDs []string,
	) (*[]Balance, error)
	GetPending(
		ctx context.Context,
		accountIDs []string,
	) (*[]PendingTransaction, error)
	GetTransactionsByAccount(
		ctx context.Context,
		accountID string,
		txTypes []string,
	) (*[]Transaction, error)
	InsertFromSettlements(
		ctx context.Context,
		txs []Settlement,
	) (sql.Result, error)
	InsertFromReferrals(
		ctx context.Context,
		txs []Referral,
	) (sql.Result, error)
	InsertFromVoting(
		ctx context.Context,
		txs []Votes,
	) (sql.Result, error)
	InsertTransactions(
		ctx context.Context,
		txs []Transaction,
	) (sql.Result, error)
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
func (pg Postgres) GetAccountEarnings(
	ctx context.Context,
	options AccountEarningsOptions,
) (*[]AccountEarnings, error) {
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
	earnings := []AccountEarnings{}
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
func (pg Postgres) GetAccountSettlementEarnings(
	ctx context.Context,
	options AccountSettlementEarningsOptions,
) (*[]AccountSettlementEarnings, error) {
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
			*options.StartDate,
			*options.UntilDate,
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
	earnings := []AccountSettlementEarnings{}

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
func (pg Postgres) GetBalances(
	ctx context.Context,
	accountIDs []string,
) (*[]Balance, error) {
	statement := `
	SELECT
		account_transactions.account_type as account_type,
		account_transactions.account_id as account_id,
		COALESCE(SUM(account_transactions.amount), 0.0) as balance
	FROM account_transactions
	WHERE account_id = any($1::text[])
	GROUP BY (account_transactions.account_id, account_transactions.account_type)`
	balances := []Balance{}

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
func (pg Postgres) GetPending(
	ctx context.Context,
	accountIDs []string,
) (*[]PendingTransaction, error) {
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
	votes := []PendingTransaction{}

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

// GetTransactionsByAccount retrieves the transactions tied to an account id
func (pg Postgres) GetTransactionsByAccount(
	ctx context.Context,
	accountID string,
	txTypes []string,
) (*[]Transaction, error) {
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
		strings.Join(transactionColumns, ", "),
		typeExtension,
	)
	transactions := []Transaction{}

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

func (pg Postgres) InsertFromSettlements(ctx context.Context, targets []Settlement) (sql.Result, error) {
	txs, err := convertToTxs(targets...)
	if err != nil {
		return nil, err
	}
	return pg.InsertTransactions(ctx, txs)
}

func (pg Postgres) InsertFromVoting(ctx context.Context, targets []Votes) (sql.Result, error) {
	txs, err := convertToTxs(targets...)
	if err != nil {
		return nil, err
	}
	return pg.InsertTransactions(ctx, txs)
}

func (pg Postgres) InsertFromReferrals(ctx context.Context, targets []Referral) (sql.Result, error) {
	txs, err := convertToTxs(targets...)
	if err != nil {
		return nil, err
	}
	return pg.InsertTransactions(ctx, txs)
}

func (pg Postgres) InsertFromUserDepositFromChain(ctx context.Context, targets []UserDeposit) (sql.Result, error) {
	txs, err := convertToTxs(targets...)
	if err != nil {
		return nil, err
	}
	return pg.InsertTransactions(ctx, txs)
}

func convertToTxs(convertables ...ConvertableTransaction) (*[]Transaction, error) {
	txs := []Transaction{}
	for _, convertable := range convertables {
		if !convertable.Valid() {
			return nil, errorutils.Wrap(
				ErrConvertableFailedValidation,
				fmt.Sprintf(
					"a convertable transaction failed validation %w",
					convertable,
				),
			)
		}
		txs = append(txs, *convertable.ToTxs()...)
	}
	return &txs, nil
}

func (pg Postgres) InsertTransactions(ctx context.Context, txs *[]Transaction) (sql.Result, error) {
	statement := fmt.Sprintf(`
INSERT INTO transactions ( %s )
VALUES ( %s )`,
		strings.Join(transactionColumns, ", "),
		strings.Join(db.ColumnsToParamNames(transactionColumns), ", "),
	)
	return sqlx.NamedExecContext(ctx, nil, statement, txs)
}

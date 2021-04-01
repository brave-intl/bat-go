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
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/getsentry/sentry-go"
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

	settlementTypes = map[string]bool{
		"contribution_settlement": true,
		"referral_settlement":     true,
	}
)

// AccountEarnings holds results from querying account earnings
type AccountEarnings struct {
	Channel   string          `json:"channel" db:"channel"`
	Earnings  decimal.Decimal `json:"earnings" db:"earnings"`
	AccountID string          `json:"account_id" db:"account_id"`
}

// AccountSettlementEarnings holds results from querying account earnings
type AccountSettlementEarnings struct {
	Channel   string          `json:"channel" db:"channel"`
	Paid      decimal.Decimal `json:"paid" db:"paid"`
	AccountID string          `json:"account_id" db:"account_id"`
}

// AccountEarningsOptions receives all options pertaining to account earnings calculations
type AccountEarningsOptions struct {
	Type      string
	Ascending bool
	Limit     int
}

// AccountSettlementEarningsOptions receives all options pertaining to account settlement earnings calculations
type AccountSettlementEarningsOptions struct {
	Type      string
	Ascending bool
	Limit     int
	StartDate *time.Time
	UntilDate *time.Time
}

// Balance holds information about an account id's balance
type Balance struct {
	AccountID string          `json:"account_id" db:"account_id"`
	Type      string          `json:"account_type" db:"account_type"`
	Balance   decimal.Decimal `json:"balance" db:"balance"`
}

// Votes holds information about an account id's balance
type Votes struct {
	Channel string          `db:"channel"`
	Balance decimal.Decimal `db:"balance"`
}

// Transaction holds info about a single transaction from the database
type Transaction struct {
	Channel            string           `db:"channel"`
	CreatedAt          time.Time        `db:"created_at"`
	Description        string           `db:"description"`
	FromAccount        string           `db:"from_account"`
	ToAccount          *string          `db:"to_account"`
	ToAccountType      *string          `db:"to_account_type"`
	Amount             decimal.Decimal  `db:"amount"`
	SettlementCurrency *string          `db:"settlement_currency"`
	SettlementAmount   *decimal.Decimal `db:"settlement_amount"`
	TransactionType    string           `db:"transaction_type"`
}

func (tx Transaction) Backfill(account string) BackfillTransaction {
	amount := tx.Amount
	if tx.FromAccount == account {
		amount = amount.Neg()
	}
	var settlementDestinationType *string
	var settlementDestination *string
	if settlementTypes[tx.TransactionType] {
		if tx.ToAccountType != nil {
			settlementDestinationType = tx.ToAccountType
		}
		if tx.ToAccount != nil {
			settlementDestination = tx.ToAccount
		}
	}
	return BackfillTransaction{
		Amount:                    inputs.Decimal{&amount},
		Channel:                   tx.Channel,
		CreatedAt:                 tx.CreatedAt,
		Description:               tx.Description,
		SettlementCurrency:        tx.SettlementCurrency,
		SettlementAmount:          &inputs.Decimal{tx.SettlementAmount},
		TransactionType:           tx.TransactionType,
		SettlementDestinationType: settlementDestinationType,
		SettlementDestination:     settlementDestination,
	}
}

// BackfillTransaction holds a backfilled version of the transaction
type BackfillTransaction struct {
	CreatedAt                 time.Time       `json:"created_at"`
	Description               string          `json:"description"`
	Channel                   string          `json:"channel"`
	Amount                    inputs.Decimal  `json:"amount"`
	SettlementCurrency        *string         `json:"settlement_currency,omitempty"`
	SettlementAmount          *inputs.Decimal `json:"settlement_amount,omitempty"`
	SettlementDestinationType *string         `json:"settlement_destination_type,omitempty"`
	SettlementDestination     *string         `json:"settlement_destination,omitempty"`
	TransactionType           string          `json:"transaction_type"`
}

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
	) (*[]Votes, error)
	GetTransactions(
		ctx context.Context,
		accountID string,
		txTypes []string,
	) (*[]Transaction, error)
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
		fmt.Sprintf("{%s}", strings.Join(accountIDs, ",")),
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
) (*[]Votes, error) {
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
	votes := []Votes{}

	err := pg.RawDB().SelectContext(
		ctx,
		&votes,
		statement,
		fmt.Sprintf("{%s}", strings.Join(accountIDs, ",")),
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return &votes, nil
}

// GetTransactions retrieves the transactions tied to an account id
func (pg Postgres) GetTransactions(
	ctx context.Context,
	accountID string,
	txTypes []string,
) (*[]Transaction, error) {
	typeExtension := ""
	args := []interface{}{accountID}
	if len(txTypes) > 0 {
		args = append(args, txTypes)
		typeExtension = "AND transaction_type = ANY($2::text[])"
	}
	statement := fmt.Sprintf(`
SELECT
  created_at,
  description,
  channel,
  amount,
  from_account,
  to_account,
  to_account_type,
  settlement_currency,
  settlement_amount,
  transaction_type
FROM transactions
WHERE (
  from_account = $1
  OR to_account = $1
) %s
ORDER BY created_at`, typeExtension)
	transactions := []Transaction{}

	err := pg.RawDB().SelectContext(
		ctx,
		&transactions,
		statement,
		accountID,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return &transactions, nil
}

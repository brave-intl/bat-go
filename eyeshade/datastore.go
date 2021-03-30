package eyeshade

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

// AccountEarnings holds results from querying account earnings
type AccountEarnings struct {
	Channel   string          `json:"channel" db:"channel"`
	Earnings  decimal.Decimal `json:"earnings" db:"earnings"`
	AccountID string          `json:"account_id" db:"account_id"`
}

// Datastore holds methods for interacting with database
type Datastore interface {
	grantserver.Datastore
	GetAccountEarnings(
		ctx context.Context,
		options AccountEarningsOptions,
	) (*[]AccountEarnings, error)
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	grantserver.Postgres
}

// AccountEarningsOptions receives all options pertaining to account earnings calculations
type AccountEarningsOptions struct {
	Type      string
	Ascending bool
	Limit     *int64
}

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
		options.Type,
		options.Limit,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return &earnings, nil
}

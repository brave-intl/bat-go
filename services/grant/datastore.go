package grant

import (
	"os"
	"time"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/datastore"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	// needed magically?

	// needed for magic migration
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Datastore abstracts over the underlying datastore
type Datastore interface {
	datastore.Datastore
	// GetGrantsOrderedByExpiry returns ordered grant claims with optional promotion type filter
	GetGrantsOrderedByExpiry(wallet walletutils.Info, promotionType string) ([]Grant, error)
}

// ReadOnlyDatastore includes all database methods that can be made with a read only db connection
type ReadOnlyDatastore interface {
	datastore.Datastore
	// GetGrantsOrderedByExpiry returns ordered grant claims with optional promotion type filter
	GetGrantsOrderedByExpiry(wallet walletutils.Info, promotionType string) ([]Grant, error)
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	datastore.Postgres
}

// NewDB creates a new Postgres Datastore
func NewDB(databaseURL string, performMigration bool, migrationTrack string, dbStatsPrefix ...string) (Datastore, error) {
	pg, err := datastore.NewPostgres(databaseURL, performMigration, migrationTrack, dbStatsPrefix...)
	if pg != nil {
		return &DatastoreWithPrometheus{
			base: &Postgres{*pg}, instanceName: "grant_datastore",
		}, err
	}
	return nil, err
}

// NewRODB creates a new Postgres RO Datastore
func NewRODB(databaseURL string, performMigration bool, migrationTrack string, dbStatsPrefix ...string) (ReadOnlyDatastore, error) {
	pg, err := datastore.NewPostgres(databaseURL, performMigration, migrationTrack, dbStatsPrefix...)
	if pg != nil {
		return &ReadOnlyDatastoreWithPrometheus{
			base: &Postgres{*pg}, instanceName: "grant_ro_datastore",
		}, err
	}
	return nil, err
}

// NewPostgres creates postgres connections
func NewPostgres() (Datastore, ReadOnlyDatastore, error) {
	var grantRoPg ReadOnlyDatastore
	grantPg, err := NewDB("", true, "grant", "grant_db")
	if err != nil {
		sentry.CaptureException(err)
		log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
	}

	roDB := os.Getenv("RO_DATABASE_URL")
	if len(roDB) > 0 {
		grantRoPg, err = NewRODB(roDB, false, "grant", "grant_read_only_db")
		if err != nil {
			sentry.CaptureException(err)
			log.Error().Err(err).Msg("Could not start reader postgres connection")
		}
	}
	return grantPg, grantRoPg, err
}

// GetGrantsOrderedByExpiry returns ordered grant claims for a wallet
func (pg *Postgres) GetGrantsOrderedByExpiry(wallet walletutils.Info, promotionType string) ([]Grant, error) {
	type GrantResult struct {
		Grant
		ApproximateValue decimal.Decimal `db:"approximate_value"`
		CreatedAt        time.Time       `db:"created_at"`
		ExpiresAt        time.Time       `db:"expires_at"`
		Platform         string          `db:"platform"`
	}

	if len(promotionType) == 0 {
		promotionType = "{ads,ugp}"
	}

	statement := `
select
	claims.id,
	claims.approximate_value,
	claims.promotion_id,
	promotions.created_at,
	promotions.expires_at,
	promotions.promotion_type,
	promotions.platform
from claims inner join promotions
on claims.promotion_id = promotions.id
where
	claims.wallet_id = $1 and
	not claims.redeemed and
	claims.legacy_claimed and
	promotions.promotion_type = any($2::text[]) and
	promotions.expires_at > now()
order by promotions.expires_at`

	var grantResults []GrantResult

	err := pg.RawDB().Select(&grantResults, statement, wallet.ID, promotionType)
	if err != nil {
		return []Grant{}, err
	}
	grants := make([]Grant, len(grantResults))

	for i, grant := range grantResults {
		{
			tmp := altcurrency.BAT
			grant.AltCurrency = &tmp
		}
		grant.Probi = grant.AltCurrency.ToProbi(grant.ApproximateValue)
		grant.MaturityTimestamp = grant.CreatedAt.Unix()
		grant.ExpiryTimestamp = grant.ExpiresAt.Unix()
		if grant.Type == "ugp" && grant.Platform == "android" {
			grant.Type = "android"
		}
		grants[i] = grant.Grant
	}

	return grants, nil
}

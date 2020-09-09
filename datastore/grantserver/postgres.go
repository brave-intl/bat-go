package grantserver

import (
	"database/sql"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/utils/metrics"
	"github.com/getsentry/sentry-go"
	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"

	// needed for magic migration
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const currentMigrationVersion = 22

var (
	// dbInstanceClassToMaxConn -  https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/AuroraPostgreSQL.Managing.html
	dbInstanceClassToMaxConn = map[string]int{
		"db.r4.large":    1600,
		"db.r4.xlarge":   3200,
		"db.r4.2xlarge":  5000,
		"db.r4.4xlarge":  5000,
		"db.r4.8xlarge":  5000,
		"db.r4.16xlarge": 5000,
		"db.r5.large":    1600,
		"db.r5.xlarge":   3300,
		"db.r5.2xlarge":  5000,
		"db.r5.4xlarge":  5000,
		"db.r5.12xlarge": 5000,
		"db.r5.24xlarge": 5000,
	}
	dbs = map[string]*sqlx.DB{}
)

// Datastore holds generic methods
type Datastore interface {
	RawDB() *sqlx.DB
	NewMigrate() (*migrate.Migrate, error)
	Migrate() error
	RollbackTxAndHandle(tx *sqlx.Tx) error
	RollbackTx(tx *sqlx.Tx)
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	*sqlx.DB
}

// RawDB - get the raw db
func (pg *Postgres) RawDB() *sqlx.DB {
	return pg.DB
}

// NewMigrate creates a Migrate instance given a Postgres instance with an active database connection
func (pg *Postgres) NewMigrate() (*migrate.Migrate, error) {
	driver, err := postgres.WithInstance(pg.RawDB().DB, &postgres.Config{})
	if err != nil {
		return nil, err
	}

	dbMigrationsURL := os.Getenv("DATABASE_MIGRATIONS_URL")
	m, err := migrate.NewWithDatabaseInstance(
		dbMigrationsURL,
		"postgres",
		driver,
	)
	if err != nil {
		return nil, err
	}

	return m, err
}

// Migrate the Postgres instance
func (pg *Postgres) Migrate() error {
	m, err := pg.NewMigrate()
	if err != nil {
		return err
	}

	err = m.Migrate(currentMigrationVersion)
	if err != migrate.ErrNoChange && err != nil {
		return err
	}
	return nil
}

// NewPostgres creates a new Postgres Datastore
func NewPostgres(databaseURL string, performMigration bool, dbStatsPrefix ...string) (*Postgres, error) {
	if len(databaseURL) == 0 {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	dbStatsPref := strings.Join(dbStatsPrefix, "_")

	key := dbStatsPref + ":" + databaseURL

	if dbs[key] != nil {
		return &Postgres{dbs[key]}, nil
	}

	db, err := sqlx.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}

	dbs[key] = db

	// setup instrumentation using sqlstats
	if len(dbStatsPrefix) > 0 {
		// Create a new collector, the name will be used as a label on the metrics
		collector := metrics.NewStatsCollector(dbStatsPref, db)
		// Register it with Prometheus
		err := prometheus.Register(collector)

		if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
			// take old collector, and add the new db
			if sc, ok := ae.ExistingCollector.(*metrics.StatsCollector); ok {
				sc.AddStatsGetter(dbStatsPref, db)
			}
		}
	}

	// if we have a connection longer than 5 minutes, kill it
	db.SetConnMaxLifetime(5 * time.Minute)

	// set max open connections to default to 80 (will get overwritten later by calculation
	// depending of if we have environment variables set
	db.SetMaxOpenConns(80)

	// using desired/max tasks to calculate the right number of max open connections
	desiredTasks, dterr := strconv.Atoi(os.Getenv("DESIRED_TASKS"))
	maxTasks, mterr := strconv.Atoi(os.Getenv("MAXIMUM_TASKS"))

	if dterr == nil && mterr == nil && desiredTasks > 0 && maxTasks > 0 {
		grantDbInstanceClass := os.Getenv("GRANT_DB_INSTANCE_CLASS")
		// 3300 / maxTasks desiredTasks
		if maxConns, ok := dbInstanceClassToMaxConn[grantDbInstanceClass]; ok {
			// if we are able to get desired tasks, max tasks and instance class from environment
			// also taking into account that payments/grants/promotions are all using the same database instance to
			// calculate the max open connections:
			db.SetMaxOpenConns(maxConns / (maxTasks / desiredTasks) / 3)
		}
	}

	pg := &Postgres{db}

	if performMigration {
		err = pg.Migrate()
		if err != nil {
			return nil, err
		}
	}

	return pg, nil
}

// RollbackTxAndHandle rolls back a transaction
func (pg *Postgres) RollbackTxAndHandle(tx *sqlx.Tx) error {
	err := tx.Rollback()
	if err != nil && err != sql.ErrTxDone {
		sentry.CaptureMessage(err.Error())
	}
	return err
}

// RollbackTx rolls back a transaction (useful with defer)
func (pg *Postgres) RollbackTx(tx *sqlx.Tx) {
	_ = pg.RollbackTxAndHandle(tx)
}

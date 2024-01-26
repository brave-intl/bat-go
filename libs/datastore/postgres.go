package datastore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/metrics"
	"github.com/getsentry/sentry-go"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"

	// needed for magic migration
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

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
	// CurrentMigrationVersion holds the default migration version
	CurrentMigrationVersion = uint(66)
	// MigrationTracks holds the migration version for a given track (eyeshade, promotion, wallet)
	MigrationTracks = map[string]uint{
		"eyeshade": 20,
	}
)

// Datastore holds generic methods
type Datastore interface {
	RawDB() *sqlx.DB
	NewMigrate() (*migrate.Migrate, error)
	Migrate(...uint) error
	RollbackTxAndHandle(tx *sqlx.Tx) error
	RollbackTx(tx *sqlx.Tx)
	BeginTx() (*sqlx.Tx, error)
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
func (pg *Postgres) Migrate(currentMigrationVersions ...uint) error {
	ctx := context.WithValue(context.Background(), appctx.EnvironmentCTXKey, os.Getenv("ENV"))
	_, logger := logging.SetupLogger(ctx)

	logger.Info().Msg("attempting database migration")

	m, err := pg.NewMigrate()
	if err != nil {
		logger.Error().Err(err).Msg("failed to create a new migration")
		return err
	}

	activeMigrationVersion, dirty, err := m.Version()

	currentMigrationVersion := CurrentMigrationVersion
	if len(currentMigrationVersions) > 0 {
		currentMigrationVersion = currentMigrationVersions[0]
	}

	subLogger := logger.With().
		Bool("dirty", dirty).
		Int("db_version", int(activeMigrationVersion)).
		Uint("code_version", currentMigrationVersion).
		Logger()

	subLogger.Info().Msg("database status")

	if !errors.Is(err, migrate.ErrNilVersion) && err != nil {
		subLogger.Error().Err(err).Msg("failed to get migration version")
		sentry.CaptureMessage(err.Error())
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	// Don't attempt the migration if our currentMigrationVersion is less than the active db version or if the migration is in dirty state
	if currentMigrationVersion < activeMigrationVersion || dirty {
		subLogger.Error().Msg("migration not attempted")
		sentry.CaptureMessage(
			fmt.Sprintf("migration not attempted, dirty: %t; code version: %d; db version: %d",
				dirty, currentMigrationVersion, activeMigrationVersion))
		return nil
	}

	err = m.Migrate(currentMigrationVersion)
	if err != migrate.ErrNoChange && err != nil {
		subLogger.Error().Err(err).Msg("migration failed")
		return err
	}

	return nil
}

// NewPostgres creates a new Postgres Datastore
func NewPostgres(
	databaseURL string,
	performMigration bool,
	migrationTrack string,
	dbStatsPrefix ...string,
) (*Postgres, error) {
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
	maxOpenConns := 80

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
			maxOpenConns = maxConns / (maxTasks / desiredTasks) / 3
		}
	}

	db.SetMaxOpenConns(maxOpenConns)
	// 50% of max open
	db.SetMaxIdleConns(maxOpenConns / 2)

	pg := &Postgres{db}

	if performMigration {
		migrationVersion := MigrationTracks[migrationTrack]
		if migrationVersion == 0 {
			migrationVersion = CurrentMigrationVersion
		}
		err = pg.Migrate(migrationVersion)
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

// BeginTx starts a transaction
func (pg *Postgres) BeginTx() (*sqlx.Tx, error) {
	return pg.RawDB().Beginx()
}

package wallet

import (
	"os"

	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	// needed for magic migration
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Datastore abstracts over the underlying datastore
type Datastore interface {
	// InsertWallet inserts the given wallet
	InsertWallet(wallet *Info) error
	// GetWallet by ID
	GetWallet(id uuid.UUID) (*Info, error)
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	*sqlx.DB
}

// NewMigrate creates a Migrate instance given a Postgres instance with an active database connection
func (pg *Postgres) NewMigrate() (*migrate.Migrate, error) {
	driver, err := postgres.WithInstance(pg.DB.DB, &postgres.Config{})
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

	err = m.Migrate(5)
	if err != migrate.ErrNoChange && err != nil {
		return err
	}
	return nil
}

// NewPostgres creates a new Postgres Datastore
func NewPostgres(databaseURL string, performMigration bool) (*Postgres, error) {
	if len(databaseURL) == 0 {
		databaseURL = os.Getenv("DATABASE_URL")
	}

	db, err := sqlx.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
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

// InsertWallet inserts the given wallet
func (pg *Postgres) InsertWallet(wallet *Info) error {
	// NOTE on conflict do nothing because none of the wallet information is updateable
	statement := `
	insert into wallets (id, provider, provider_id, public_key)
	values ($1, $2, $3, $4)
	on conflict do nothing
	returning *`
	_, err := pg.DB.Exec(
		statement,
		wallet.ID,
		wallet.Provider,
		wallet.ProviderID,
		wallet.PublicKey,
	)
	if err != nil {
		return err
	}

	return nil
}

// GetWallet by ID
func (pg *Postgres) GetWallet(ID uuid.UUID) (*Info, error) {
	statement := "select * from wallets where id = $1"
	wallets := []Info{}
	err := pg.DB.Select(&wallets, statement, ID)
	if err != nil {
		return nil, err
	}

	if len(wallets) > 0 {
		return &wallets[0], nil
	}

	return nil, nil
}

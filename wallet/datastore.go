package wallet

import (
	"os"

	"github.com/getsentry/sentry-go"
	"github.com/pkg/errors"

	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"

	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"

	// needed for magic migration
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Datastore holds the interface for the wallet datastore
type Datastore interface {
	SetAnonymousAddress(ID uuid.UUID, anonymousAddress uuid.UUID) error
	TxLinkWalletInfo(tx *sqlx.Tx, ID uuid.UUID, providerLinkingID uuid.UUID, anonymousAddress uuid.UUID) error
	TxGetByProviderLinkingID(tx *sqlx.Tx, providerLinkingID uuid.UUID) (*[]Info, error)
	LinkWallet(id uuid.UUID, providerLinkingID uuid.UUID, anonymousAddress uuid.UUID) error
	// GetWallet by ID
	GetWallet(id uuid.UUID) (*Info, error)
	// InsertWallet inserts the given wallet
	InsertWallet(wallet *Info) error
}

// ReadOnlyDatastore includes all database methods that can be made with a read only db connection
type ReadOnlyDatastore interface {
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

// GetWallet by ID
func (pg *Postgres) GetWallet(ID uuid.UUID) (*Info, error) {
	statement := "SELECT * FROM wallets WHERE id = $1"
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

// SetAnonymousAddress sets the anon addresses of the provided wallets
func (pg *Postgres) SetAnonymousAddress(ID uuid.UUID, anonymousAddress uuid.UUID) error {
	statement := `
	UPDATE wallets
	SET
			anonymous_address = $2
	WHERE id = $1
	`
	_, err := pg.DB.Exec(
		statement,
		ID.String(),
		anonymousAddress.String(),
	)
	return err
}

// InsertWallet inserts the given wallet
func (pg *Postgres) InsertWallet(wallet *Info) error {
	// NOTE on conflict do nothing because none of the wallet information is updateable
	statement := `
	INSERT INTO wallets (id, provider, provider_id, public_key)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT DO NOTHING;`
	_, err := pg.DB.Exec(statement, wallet.ID, wallet.Provider, wallet.ProviderID, wallet.PublicKey)
	if err != nil {
		return err
	}

	return nil
}

// TxLinkWalletInfo pass a tx to set the anonymous address
func (pg *Postgres) TxLinkWalletInfo(tx *sqlx.Tx, ID uuid.UUID, providerLinkingID uuid.UUID, anonymousAddress uuid.UUID) error {
	statement := `
	UPDATE wallets
	SET
			provider_linking_id = $2,
			anonymous_address = $3
	WHERE id = $1;`
	_, err := tx.Exec(
		statement,
		ID.String(),
		providerLinkingID.String(),
		anonymousAddress.String(),
	)
	return err
}

// TxGetByProviderLinkingID gets a wallet by a provider address
func (pg *Postgres) TxGetByProviderLinkingID(tx *sqlx.Tx, providerLinkingID uuid.UUID) (*[]Info, error) {
	statement := `
	SELECT *
	FROM wallets
	WHERE provider_linking_id = $1
	`
	var wallets []Info
	err := tx.Select(&wallets, statement, providerLinkingID.String())
	if err != nil {
		return nil, err
	}
	return &wallets, nil
}

// LinkWallet links a wallet together
func (pg *Postgres) LinkWallet(ID uuid.UUID, providerLinkingID uuid.UUID, anonymousAddress uuid.UUID) error {
	tx, err := pg.DB.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	walletsMatchingProviderLinkingID, err := pg.TxGetByProviderLinkingID(tx, providerLinkingID)
	if err != nil {
		return errors.Wrap(err, "error looking up wallets by provider id")
	}
	walletLinkedLength := len(*walletsMatchingProviderLinkingID)
	if walletLinkedLength >= 3 {
		if walletLinkedLength > 3 {
			sentry.WithScope(func(scope *sentry.Scope) {
				scope.SetTags(map[string]string{
					"walletId":          ID.String(),
					"providerLinkingId": providerLinkingID.String(),
					"anonymousAddress":  anonymousAddress.String(),
				})
				sentry.CaptureMessage("too many cards linked")
			})
		}
		return errors.New("unable to add too many wallets to a single user")
	}
	err = pg.TxLinkWalletInfo(tx, ID, providerLinkingID, anonymousAddress)
	if err != nil {
		return errors.Wrap(err, "unable to set an anonymous address")
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

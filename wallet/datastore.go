package wallet

import (
	"errors"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/getsentry/sentry-go"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"

	// needed for magic migration
	"github.com/golang-migrate/migrate"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

var (
	// ErrTooManyCardsLinked denotes when more than 3 cards have been linked to a single wallet
	ErrTooManyCardsLinked = errors.New("unable to add too many wallets to a single user")
)

// Datastore holds the interface for the wallet datastore
type Datastore interface {
	SetAnonymousAddress(ID string, anonymousAddress *uuid.UUID) error
	TxLinkWalletInfo(tx *sqlx.Tx, ID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID) error
	GetByProviderLinkingID(providerLinkingID uuid.UUID) (*[]walletutils.Info, error)
	txGetByProviderLinkingID(tx *sqlx.Tx, providerLinkingID uuid.UUID) (*[]walletutils.Info, error)
	LinkWallet(ID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID) error
	// GetWallet by ID
	GetWallet(ID uuid.UUID) (*walletutils.Info, error)
	// InsertWallet inserts the given wallet
	InsertWallet(wallet *walletutils.Info) error
	// UpsertWallets inserts a wallet if it does not already exist
	UpsertWallet(wallet *walletutils.Info) error
	// RawDB returns the db
	RawDB() *sqlx.DB
	// NewMigrate
	NewMigrate() (*migrate.Migrate, error)
	// Migrate
	Migrate() error
	// RollbackTx
	RollbackTx(tx *sqlx.Tx)
}

// ReadOnlyDatastore includes all database methods that can be made with a read only db connection
type ReadOnlyDatastore interface {
	// RawDB returns the db
	RawDB() *sqlx.DB
	// GetWallet by ID
	GetWallet(ID uuid.UUID) (*walletutils.Info, error)
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	grantserver.Postgres
}

// NewPostgres creates a new Postgres Datastore
func NewPostgres(databaseURL string, performMigration bool, dbStatsPrefix ...string) (Datastore, error) {
	pg, err := grantserver.NewPostgres(databaseURL, performMigration, dbStatsPrefix...)
	if pg != nil {
		return &DatastoreWithPrometheus{
			base: &Postgres{*pg}, instanceName: "wallet_datastore",
		}, err
	}
	return nil, err
}

// NewROPostgres creates a new Postgres RO Datastore
func NewROPostgres(databaseURL string, performMigration bool, dbStatsPrefix ...string) (ReadOnlyDatastore, error) {
	pg, err := grantserver.NewPostgres(databaseURL, performMigration, dbStatsPrefix...)
	if pg != nil {
		return &ReadOnlyDatastoreWithPrometheus{
			base: &Postgres{*pg}, instanceName: "wallet_ro_datastore",
		}, err
	}
	return nil, err
}

// GetWallet by ID
func (pg *Postgres) GetWallet(ID uuid.UUID) (*walletutils.Info, error) {
	return pg.Postgres.GetWallet(ID)
}

// UpsertWallet inserts a wallet if one does not already exist
func (pg *Postgres) UpsertWallet(wallet *walletutils.Info) error {
	return pg.Postgres.UpsertWallet(wallet)
}

// SetAnonymousAddress sets the anon addresses of the provided wallets
func (pg *Postgres) SetAnonymousAddress(ID string, anonymousAddress *uuid.UUID) error {
	statement := `
	UPDATE wallets
	SET
			anonymous_address = $2
	WHERE id = $1
	`
	_, err := pg.DB.Exec(
		statement,
		ID,
		anonymousAddress,
	)
	return err
}

// InsertWallet inserts the given wallet
func (pg *Postgres) InsertWallet(wallet *walletutils.Info) error {
	// NOTE on conflict do nothing because none of the wallet information is updateable
	statement := `
	INSERT INTO wallets (id, provider, provider_id, public_key)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT DO NOTHING`
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

// TxLinkWalletInfo pass a tx to set the anonymous address
func (pg *Postgres) TxLinkWalletInfo(tx *sqlx.Tx, ID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID) error {
	statement := `
	UPDATE wallets
	SET
			provider_linking_id = $2,
			anonymous_address = $3
	WHERE id = $1;`
	_, err := tx.Exec(
		statement,
		ID,
		providerLinkingID,
		anonymousAddress,
	)
	return err
}

func (pg *Postgres) txGetByProviderLinkingID(tx *sqlx.Tx, providerLinkingID uuid.UUID) (*[]walletutils.Info, error) {
	statement := `
	SELECT *
	FROM wallets
	WHERE provider_linking_id = $1
	`
	var wallets []walletutils.Info
	err := tx.Select(&wallets, statement, providerLinkingID)
	return &wallets, err
}

// GetByProviderLinkingID gets a wallet by a provider address
func (pg *Postgres) GetByProviderLinkingID(providerLinkingID uuid.UUID) (*[]walletutils.Info, error) {
	statement := `
	SELECT *
	FROM wallets
	WHERE provider_linking_id = $1
	`
	var wallets []walletutils.Info
	err := pg.DB.Select(&wallets, statement, providerLinkingID)
	return &wallets, err
}

// LinkWallet links a wallet together
func (pg *Postgres) LinkWallet(ID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID) error {
	tx, err := pg.DB.Beginx()
	if err != nil {
		return err
	}
	defer pg.RollbackTx(tx)

	walletsMatchingProviderLinkingID, err := pg.txGetByProviderLinkingID(tx, providerLinkingID)
	if err != nil {
		return errorutils.Wrap(err, "error looking up wallets by provider id")
	}
	walletLinkedLength := len(*walletsMatchingProviderLinkingID)
	if walletLinkedLength >= 3 {
		if walletLinkedLength > 3 {
			sentry.WithScope(func(scope *sentry.Scope) {
				anonAddr := ""
				if anonymousAddress != nil {
					anonAddr = anonymousAddress.String()
				}
				scope.SetTags(map[string]string{
					"walletId":          ID,
					"providerLinkingId": providerLinkingID.String(),
					"anonymousAddress":  anonAddr,
				})
				sentry.CaptureMessage("too many cards linked")
			})
		}
		return ErrTooManyCardsLinked
	}
	err = pg.TxLinkWalletInfo(tx, ID, providerLinkingID, anonymousAddress)
	if err != nil {
		return errorutils.Wrap(err, "unable to set an anonymous address")
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

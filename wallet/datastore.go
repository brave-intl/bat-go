package wallet

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/wallet"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/getsentry/sentry-go"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"

	// needed for magic migration
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

var (
	tooManyCardsCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:        "too_many_linked_cards",
			Help:        "A counter for too many linked cards",
			ConstLabels: prometheus.Labels{"service": "wallet"},
		})
)

func init() {
	prometheus.MustRegister(tooManyCardsCounter)
}

// Datastore holds the interface for the wallet datastore
type Datastore interface {
	grantserver.Datastore
	TxLinkWalletInfo(tx *sqlx.Tx, ID string, providerID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID, pda string) error
	LinkWallet(ID string, providerID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID, depositProvider string) error
	// GetByProviderLinkingID gets the wallet by provider linking id
	GetByProviderLinkingID(providerLinkingID uuid.UUID) (*[]walletutils.Info, error)
	// GetWallet by ID
	GetWallet(ID uuid.UUID) (*walletutils.Info, error)
	// GetWalletByPublicKey by ID
	GetWalletByPublicKey(string) (*walletutils.Info, error)
	// InsertWallet inserts the given wallet
	InsertWallet(wallet *walletutils.Info) error
	// UpsertWallets inserts a wallet if it does not already exist
	UpsertWallet(wallet *walletutils.Info) error
}

// ReadOnlyDatastore includes all database methods that can be made with a read only db connection
type ReadOnlyDatastore interface {
	grantserver.Datastore
	// GetByProviderLinkingID gets a wallet by provider linking id
	GetByProviderLinkingID(providerLinkingID uuid.UUID) (*[]walletutils.Info, error)
	// GetWallet by ID
	GetWallet(ID uuid.UUID) (*walletutils.Info, error)
	// GetWalletByPublicKey
	GetWalletByPublicKey(string) (*walletutils.Info, error)
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	grantserver.Postgres
}

// NewWritablePostgres creates a new Postgres Datastore
func NewWritablePostgres(databaseURL string, performMigration bool, dbStatsPrefix ...string) (Datastore, error) {
	pg, err := grantserver.NewPostgres(databaseURL, performMigration, dbStatsPrefix...)
	if pg != nil {
		return &DatastoreWithPrometheus{
			base: &Postgres{*pg}, instanceName: "wallet_datastore",
		}, err
	}
	return nil, err
}

// NewReadOnlyPostgres creates a new Postgres RO Datastore
func NewReadOnlyPostgres(databaseURL string, performMigration bool, dbStatsPrefix ...string) (ReadOnlyDatastore, error) {
	pg, err := grantserver.NewPostgres(databaseURL, performMigration, dbStatsPrefix...)
	if pg != nil {
		return &ReadOnlyDatastoreWithPrometheus{
			base: &Postgres{*pg}, instanceName: "wallet_ro_datastore",
		}, err
	}
	return nil, err
}

// NewPostgres creates postgres connections
func NewPostgres() (Datastore, ReadOnlyDatastore, error) {
	var walletRODB ReadOnlyDatastore
	walletDB, err := NewWritablePostgres("", true)
	if err != nil {
		sentry.CaptureException(err)
		log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
		return nil, nil, err
	}
	roDB := os.Getenv("RO_DATABASE_URL")
	if len(roDB) > 0 {
		walletRODB, err = NewReadOnlyPostgres(roDB, false)
		if err != nil {
			sentry.CaptureException(err)
			log.Error().Err(err).Msg("Could not start reader postgres connection")
			return nil, nil, err
		}
	}
	return walletDB, walletRODB, nil
}

var (
	// ErrTooManyCardsLinked denotes when more than 3 cards have been linked to a single wallet
	ErrTooManyCardsLinked = errors.New("unable to add too many wallets to a single user")
)

// UpsertWallet upserts the given wallet
func (pg *Postgres) UpsertWallet(wallet *wallet.Info) error {
	statement := `
	insert into wallets
		(
			id, provider, provider_id, public_key, provider_linking_id, anonymous_address,
			user_deposit_account_provider, user_deposit_destination
		)
	values
		($1, $2, $3, $4, $5, $6, $7, $8)
	on conflict (id) do
	update set
		provider = $2,
		provider_id = $3,
		provider_linking_id = $5,
		anonymous_address = $6,
		user_deposit_account_provider = $7,
		user_deposit_destination = $8
	returning *`
	_, err := pg.RawDB().Exec(statement, wallet.ID, wallet.Provider, wallet.ProviderID, wallet.PublicKey, wallet.ProviderLinkingID, wallet.AnonymousAddress, wallet.UserDepositAccountProvider, wallet.UserDepositDestination)
	if err != nil {
		return err
	}

	return nil
}

// GetWallet by ID
func (pg *Postgres) GetWallet(ID uuid.UUID) (*wallet.Info, error) {
	statement := `
	select
		id, provider, provider_id, public_key, provider_linking_id, anonymous_address,
		user_deposit_account_provider, user_deposit_destination
	from
		wallets
	where
		id = $1`
	wallets := []wallet.Info{}
	err := pg.RawDB().Select(&wallets, statement, ID)
	if err != nil {
		return nil, err
	}

	if len(wallets) > 0 {
		// FIXME currently assumes BAT
		{
			tmp := altcurrency.BAT
			wallets[0].AltCurrency = &tmp
		}
		return &wallets[0], nil
	}

	return nil, nil
}

// txGetWallet by ID
func (pg *Postgres) txHasDestination(tx *sqlx.Tx, ID uuid.UUID) (bool, error) {
	statement := `
	select
		true
	from
		wallets
	where
		user_deposit_destination is not null and 
		user_deposit_destination != '' and
		id = $1`
	var result bool
	err := tx.Get(&result, statement, ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	return result, nil
}

// GetWalletByPublicKey gets a wallet by a public key
func (pg *Postgres) GetWalletByPublicKey(pk string) (*walletutils.Info, error) {
	statement := `
	select
		id, provider, provider_id, public_key, provider_linking_id, anonymous_address,
		user_deposit_account_provider, user_deposit_destination
	from
		wallets
	WHERE public_key = $1
	`
	var wallet walletutils.Info
	err := pg.RawDB().Get(&wallet, statement, pk)
	return &wallet, err
}

// GetByProviderLinkingID gets a wallet by a provider address
func (pg *Postgres) GetByProviderLinkingID(providerLinkingID uuid.UUID) (*[]walletutils.Info, error) {
	statement := `
	select
		id, provider, provider_id, public_key, provider_linking_id, anonymous_address,
		user_deposit_account_provider, user_deposit_destination
	from
		wallets
	WHERE provider_linking_id = $1
	`
	var wallets []walletutils.Info
	err := pg.RawDB().Select(&wallets, statement, providerLinkingID)
	return &wallets, err
}

// InsertWallet inserts the given wallet
func (pg *Postgres) InsertWallet(wallet *walletutils.Info) error {
	// NOTE on conflict do nothing because none of the wallet information is updateable
	statement := `
	INSERT INTO wallets (id, provider, provider_id, public_key)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT DO NOTHING`
	_, err := pg.RawDB().Exec(
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
func (pg *Postgres) TxLinkWalletInfo(
	tx *sqlx.Tx,
	ID string,
	userDepositDestination string,
	providerLinkingID uuid.UUID,
	anonymousAddress *uuid.UUID,
	userDepositAccountProvider string) error {

	var (
		statement string
		sqlErr    error
		id        = uuid.Must(uuid.FromString(ID))
		r         sql.Result
	)

	if ok, err := pg.txHasDestination(tx, id); err != nil {
		return fmt.Errorf("error trying to lookup anonymous address: %w", err)
	} else if ok {
		statement = `
			UPDATE wallets
			SET
				provider_linking_id = $2,
				user_deposit_account_provider = $3,
			WHERE id = $1;`
		r, sqlErr = tx.Exec(
			statement,
			ID,
			providerLinkingID,
			userDepositAccountProvider,
		)
	} else {
		statement = `
			UPDATE wallets
			SET
					provider_linking_id = $2,
					anonymous_address = $3,
					user_deposit_account_provider = $4,
					user_deposit_destination = $5
			WHERE id = $1;`
		r, sqlErr = tx.Exec(
			statement,
			ID,
			providerLinkingID,
			anonymousAddress,
			userDepositAccountProvider,
			userDepositDestination,
		)
	}

	if sqlErr != nil {
		return sqlErr
	}
	if r != nil {
		count, _ := r.RowsAffected()
		if count < 1 {
			return errors.New("should have updated at least one wallet")
		}
	}
	return nil
}

func txGetByProviderLinkingID(tx *sqlx.Tx, providerLinkingID uuid.UUID) (*[]walletutils.Info, error) {
	statement := `
	select
		id, provider, provider_id, public_key, provider_linking_id, anonymous_address,
		user_deposit_account_provider, user_deposit_destination
	from
		wallets
	WHERE provider_linking_id = $1
	`
	var wallets []walletutils.Info
	err := tx.Select(&wallets, statement, providerLinkingID)
	return &wallets, err
}

func getEnvMaxCards() int {
	if v, err := strconv.Atoi(os.Getenv("UPHOLD_WALLET_LINKING_LIMIT")); err == nil {
		return v
	}
	return 4
}

// LinkWallet links a wallet together
func (pg *Postgres) LinkWallet(ID string, userDepositDestination string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID, depositProvider string) error {
	tx, err := pg.RawDB().Beginx()
	if err != nil {
		return err
	}
	defer pg.RollbackTx(tx)

	walletsMatchingProviderLinkingID, err := txGetByProviderLinkingID(tx, providerLinkingID)
	if err != nil {
		return errorutils.Wrap(err, "error looking up wallets by provider id")
	}
	walletLinkedLength := len(*walletsMatchingProviderLinkingID)
	if walletLinkedLength >= getEnvMaxCards() {
		sentry.WithScope(func(scope *sentry.Scope) {
			anonAddr := ""
			if anonymousAddress != nil {
				anonAddr = anonymousAddress.String()
			}
			scope.SetTags(map[string]string{
				"walletId":               ID,
				"providerLinkingId":      providerLinkingID.String(),
				"anonymousAddress":       anonAddr,
				"userDepositDestination": userDepositDestination,
			})
			tooManyCardsCounter.Inc()
		})
		return ErrTooManyCardsLinked
	}

	err = pg.TxLinkWalletInfo(tx, ID, userDepositDestination, providerLinkingID, anonymousAddress, depositProvider)
	if err != nil {
		return errorutils.Wrap(err, "unable to set an anonymous address")
	}

	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

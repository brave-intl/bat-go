package wallet

import (
	"errors"
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
	SetAnonymousAddress(ID string, anonymousAddress *uuid.UUID) error
	TxLinkWalletInfo(tx *sqlx.Tx, ID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID) error
	TxSetDepositProvider(tx *sqlx.Tx, ID string, provider string) error
	LinkWallet(ID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID, depositProvider string) error
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
			user_deposit_account_provider
		)
	values
		($1, $2, $3, $4, $5, $6, $7)
	on conflict (id) do
	update set
		provider = $2,
		provider_id = $3,
		provider_linking_id = $5,
		anonymous_address = $6,
		user_deposit_account_provider = $7
	returning *`
	_, err := pg.RawDB().Exec(statement, wallet.ID, wallet.Provider, wallet.ProviderID, wallet.PublicKey, wallet.ProviderLinkingID, wallet.AnonymousAddress, wallet.UserDepositAccountProvider)
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
		user_deposit_account_provider
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

// GetWalletByPublicKey gets a wallet by a public key
func (pg *Postgres) GetWalletByPublicKey(pk string) (*walletutils.Info, error) {
	statement := `
	select
		id, provider, provider_id, public_key, provider_linking_id, anonymous_address,
		user_deposit_account_provider
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
		user_deposit_account_provider
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

// SetAnonymousAddress sets the anon addresses of the provided wallets
func (pg *Postgres) SetAnonymousAddress(ID string, anonymousAddress *uuid.UUID) error {
	statement := `
	UPDATE wallets
	SET
			anonymous_address = $2
	WHERE id = $1
	`
	_, err := pg.RawDB().Exec(
		statement,
		ID,
		anonymousAddress,
	)
	return err
}

// TxSetDepositProvider pass a tx to set the anonymous address
func (pg *Postgres) TxSetDepositProvider(tx *sqlx.Tx, ID string, provider string) error {
	statement := `
	UPDATE wallets
	SET
			user_account_deposit_provider = $2
	WHERE id = $1;`
	r, err := tx.Exec(
		statement,
		ID,
		provider,
	)
	count, _ := r.RowsAffected()
	if count < 1 {
		return errors.New("should have updated at least one wallet")
	}
	return err
}

// TxLinkWalletInfo pass a tx to set the anonymous address
func (pg *Postgres) TxLinkWalletInfo(tx *sqlx.Tx, ID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID) error {
	statement := `
	UPDATE wallets
	SET
			provider_linking_id = $2,
			anonymous_address = $3
	WHERE id = $1;`
	r, err := tx.Exec(
		statement,
		ID,
		providerLinkingID,
		anonymousAddress,
	)
	count, _ := r.RowsAffected()
	if count < 1 {
		return errors.New("should have updated at least one wallet")
	}
	return err
}

func txGetByProviderLinkingID(tx *sqlx.Tx, providerLinkingID uuid.UUID) (*[]walletutils.Info, error) {
	statement := `
	select
		id, provider, provider_id, public_key, provider_linking_id, anonymous_address,
		user_deposit_account_provider
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
func (pg *Postgres) LinkWallet(ID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID, depositProvider string) error {
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
				"walletId":          ID,
				"providerLinkingId": providerLinkingID.String(),
				"anonymousAddress":  anonAddr,
			})
			tooManyCardsCounter.Inc()
		})
		return ErrTooManyCardsLinked
	}
	err = pg.TxLinkWalletInfo(tx, ID, providerLinkingID, anonymousAddress)
	if err != nil {
		return errorutils.Wrap(err, "unable to set an anonymous address")
	}

	if depositProvider != "" {
		err = pg.TxSetDepositProvider(tx, ID, depositProvider)
		if err != nil {
			return errorutils.Wrap(err, "unable to set a deposit provider")
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

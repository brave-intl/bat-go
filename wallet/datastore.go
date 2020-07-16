package wallet

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/wallet"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/getsentry/sentry-go"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"

	// needed for magic migration
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Datastore holds the interface for the wallet datastore
type Datastore interface {
	grantserver.Datastore
	SetAnonymousAddress(ID string, anonymousAddress *uuid.UUID) error
	TxLinkWalletInfo(tx *sqlx.Tx, ID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID) error
	LinkWallet(ID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID) error
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
		sentry.Flush(time.Second * 2)
		log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
		return nil, nil, err
	}
	roDB := os.Getenv("RO_DATABASE_URL")
	if len(roDB) > 0 {
		walletRODB, err = NewReadOnlyPostgres(roDB, false)
		if err != nil {
			sentry.CaptureException(err)
			sentry.Flush(time.Second * 2)
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
	insert into wallets (id, provider, provider_id, public_key, provider_linking_id, anonymous_address)
	values ($1, $2, $3, $4, $5, $6)
	on conflict (id) do
	update set
		provider = $2,
		provider_id = $3,
		provider_linking_id = $5,
		anonymous_address = $6
	returning *`
	_, err := pg.RawDB().Exec(statement, wallet.ID, wallet.Provider, wallet.ProviderID, wallet.PublicKey, wallet.ProviderLinkingID, wallet.AnonymousAddress)
	if err != nil {
		return err
	}

	return nil
}

// GetWallet by ID
func (pg *Postgres) GetWallet(ID uuid.UUID) (*wallet.Info, error) {
	statement := "select * from wallets where id = $1"
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
	SELECT *
	FROM wallets
	WHERE public_key = $1
	`
	var wallet walletutils.Info
	err := pg.RawDB().Get(&wallet, statement, pk)
	return &wallet, err
}

// GetByProviderLinkingID gets a wallet by a provider address
func (pg *Postgres) GetByProviderLinkingID(providerLinkingID uuid.UUID) (*[]walletutils.Info, error) {
	statement := `
	SELECT *
	FROM wallets
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

// LinkWallet links a wallet and transfers funds to newly linked wallet
func (service *Service) LinkWallet(
	ctx context.Context,
	info *walletutils.Info,
	transaction string,
	anonymousAddress *uuid.UUID,
) error {
	// do not confirm this transaction yet
	tx, err := service.SubmitCommitableAnonCardTransaction(
		ctx,
		info,
		transaction,
		"",
		false,
	)
	if err != nil {
		return handlers.WrapError(err, "unable to verify transaction", http.StatusBadRequest)
	}
	if tx.UserID == "" {
		err := errors.New("user id not provided")
		return handlers.WrapError(err, "unable to link wallet", http.StatusBadRequest)
	}
	providerLinkingID := uuid.NewV5(walletClaimNamespace, tx.UserID)
	if info.ProviderLinkingID != nil {
		// check if the member matches the associated member
		if !uuid.Equal(*info.ProviderLinkingID, providerLinkingID) {
			return handlers.WrapError(errors.New("wallets do not match"), "unable to match wallets", http.StatusForbidden)
		}
		if anonymousAddress != nil && info.AnonymousAddress != nil && !uuid.Equal(*anonymousAddress, *info.AnonymousAddress) {
			err := service.Datastore.SetAnonymousAddress(info.ID, anonymousAddress)
			if err != nil {
				return handlers.WrapError(err, "unable to set anonymous address", http.StatusInternalServerError)
			}
		}
	} else {
		err := service.Datastore.LinkWallet(info.ID, providerLinkingID, anonymousAddress)
		if err != nil {
			status := http.StatusInternalServerError
			if err == ErrTooManyCardsLinked {
				status = http.StatusConflict
			}
			return handlers.WrapError(err, "unable to link wallets", status)
		}
	}

	if decimal.NewFromFloat(0).LessThan(tx.Probi) {
		_, err := service.SubmitCommitableAnonCardTransaction(ctx, info, transaction, "", true)
		if err != nil {
			return handlers.WrapError(err, "unable to transfer tokens", http.StatusBadRequest)
		}
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

func txGetByProviderLinkingID(tx *sqlx.Tx, providerLinkingID uuid.UUID) (*[]walletutils.Info, error) {
	statement := `
	SELECT *
	FROM wallets
	WHERE provider_linking_id = $1
	`
	var wallets []walletutils.Info
	err := tx.Select(&wallets, statement, providerLinkingID)
	return &wallets, err
}

// LinkWallet links a wallet together
func (pg *Postgres) LinkWallet(ID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID) error {
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

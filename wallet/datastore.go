package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/logging"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/getsentry/sentry-go"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
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
	TxLinkWalletInfo(ctx context.Context, tx *sqlx.Tx, ID string, providerID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID, pda string) error
	LinkWallet(ctx context.Context, ID string, providerID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID, depositProvider string) error
	IncreaseLinkingLimit(ctx context.Context, providerLinkingID uuid.UUID) error
	GetLinkingLimitInfo(ctx context.Context, providerLinkingID string) (LinkingInfo, error)
	// GetByProviderLinkingID gets the wallet by provider linking id
	GetByProviderLinkingID(ctx context.Context, providerLinkingID uuid.UUID) (*[]walletutils.Info, error)
	// GetWallet by ID
	GetWallet(ctx context.Context, ID uuid.UUID) (*walletutils.Info, error)
	// GetWalletByPublicKey by ID
	GetWalletByPublicKey(context.Context, string) (*walletutils.Info, error)
	// InsertWallet inserts the given wallet
	InsertWallet(ctx context.Context, wallet *walletutils.Info) error
	// InsertBitFlyerRequestID - attempt an insert on a request id
	InsertBitFlyerRequestID(ctx context.Context, requestID string) error
	// UpsertWallets inserts a wallet if it does not already exist
	UpsertWallet(ctx context.Context, wallet *walletutils.Info) error
	// ConnectCustodialWallet - connect the wallet's custodial verified wallet.
	ConnectCustodialWallet(ctx context.Context, cl CustodianLink) error
	// InsertCustodianLink - create a record of a custodian wallet
	InsertCustodianLink(ctx context.Context, cl *CustodianLink) error
	// DisconnectCustodialWallet - disconnect the wallet's custodial id
	DisconnectCustodialWallet(ctx context.Context, walletID uuid.UUID) error
	// GetCustodianLinkByID - get the custodian link by ID
	GetCustodianLinkByID(ctx context.Context, ID uuid.UUID) (*CustodianLink, error)
	// GetCustodianLinkByWalletID - get the custodian link by ID
	GetCustodianLinkByWalletID(ctx context.Context, ID uuid.UUID) (*CustodianLink, error)
	// GetCustodianLinkCount - get the wallet custodian link count across all wallets
	GetCustodianLinkCount(ctx context.Context, linkingID uuid.UUID) (int, int, error)
}

// ReadOnlyDatastore includes all database methods that can be made with a read only db connection
type ReadOnlyDatastore interface {
	grantserver.Datastore
	// GetByProviderLinkingID gets a wallet by provider linking id
	GetByProviderLinkingID(ctx context.Context, providerLinkingID uuid.UUID) (*[]walletutils.Info, error)
	// GetWallet by ID
	GetWallet(ctx context.Context, ID uuid.UUID) (*walletutils.Info, error)
	// GetWalletByPublicKey
	GetWalletByPublicKey(context.Context, string) (*walletutils.Info, error)
	// GetCustodianLinkByID - get the custodian link by ID
	GetCustodianLinkByID(ctx context.Context, ID uuid.UUID) (*CustodianLink, error)
	// GetCustodianLinkByWalletID - get the custodian link by ID
	GetCustodianLinkByWalletID(ctx context.Context, ID uuid.UUID) (*CustodianLink, error)
	// GetCustodianLinkCount - get the wallet custodian link count across all wallets
	GetCustodianLinkCount(ctx context.Context, linkingID uuid.UUID) (int, int, error)
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	grantserver.Postgres
}

// NewWritablePostgres creates a new Postgres Datastore
func NewWritablePostgres(databaseURL string, performMigration bool, migrationTrack string, dbStatsPrefix ...string) (Datastore, error) {
	pg, err := grantserver.NewPostgres(databaseURL, performMigration, migrationTrack, dbStatsPrefix...)
	if pg != nil {
		return &DatastoreWithPrometheus{
			base: &Postgres{*pg}, instanceName: "wallet_datastore",
		}, err
	}
	return nil, err
}

// NewReadOnlyPostgres creates a new Postgres RO Datastore
func NewReadOnlyPostgres(databaseURL string, performMigration bool, migrationTrack string, dbStatsPrefix ...string) (ReadOnlyDatastore, error) {
	pg, err := grantserver.NewPostgres(databaseURL, performMigration, migrationTrack, dbStatsPrefix...)
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
	walletDB, err := NewWritablePostgres("", true, "wallet")
	if err != nil {
		sentry.CaptureException(err)
		log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
		return nil, nil, err
	}
	roDB := os.Getenv("RO_DATABASE_URL")
	if len(roDB) > 0 {
		walletRODB, err = NewReadOnlyPostgres(roDB, false, "wallet")
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
func (pg *Postgres) UpsertWallet(ctx context.Context, wallet *walletutils.Info) error {
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
	_, err := pg.RawDB().ExecContext(ctx, statement, wallet.ID, wallet.Provider, wallet.ProviderID, wallet.PublicKey, wallet.ProviderLinkingID, wallet.AnonymousAddress, wallet.UserDepositAccountProvider, wallet.UserDepositDestination)
	if err != nil {
		return err
	}

	return nil
}

// GetWallet by ID
func (pg *Postgres) GetWallet(ctx context.Context, ID uuid.UUID) (*walletutils.Info, error) {
	statement := `
	select
		id, provider, provider_id, public_key, provider_linking_id, anonymous_address,
		user_deposit_account_provider, user_deposit_destination
	from
		wallets
	where
		id = $1`
	wallets := []walletutils.Info{}
	err := pg.RawDB().SelectContext(ctx, &wallets, statement, ID)
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
func (pg *Postgres) txHasDestination(ctx context.Context, tx *sqlx.Tx, ID uuid.UUID) (bool, error) {
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
	err := tx.GetContext(ctx, &result, statement, ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	return result, nil
}

// GetWalletByPublicKey gets a wallet by a public key
func (pg *Postgres) GetWalletByPublicKey(ctx context.Context, pk string) (*walletutils.Info, error) {
	statement := `
	select
		id, provider, provider_id, public_key, provider_linking_id, anonymous_address,
		user_deposit_account_provider, user_deposit_destination
	from
		wallets
	WHERE public_key = $1
	`
	var wallet walletutils.Info
	err := pg.RawDB().GetContext(ctx, &wallet, statement, pk)
	return &wallet, err
}

// GetByProviderLinkingID gets a wallet by a provider address
func (pg *Postgres) GetByProviderLinkingID(ctx context.Context, providerLinkingID uuid.UUID) (*[]walletutils.Info, error) {
	statement := `
	select
		id, provider, provider_id, public_key, provider_linking_id, anonymous_address,
		user_deposit_account_provider, user_deposit_destination
	from
		wallets
	WHERE provider_linking_id = $1
	`
	var wallets []walletutils.Info
	err := pg.RawDB().SelectContext(ctx, &wallets, statement, providerLinkingID)
	return &wallets, err
}

// InsertBitFlyerRequestID - attempts to insert a request id
func (pg *Postgres) InsertBitFlyerRequestID(ctx context.Context, requestID string) error {
	statement := `insert into bf_req_ids(id) values ($1)`
	_, err := pg.RawDB().ExecContext(ctx, statement, requestID)
	if err != nil {
		return err
	}

	return nil

}

// InsertWallet inserts the given wallet
func (pg *Postgres) InsertWallet(ctx context.Context, wallet *walletutils.Info) error {
	// NOTE on conflict do nothing because none of the wallet information is updateable
	statement := `
	INSERT INTO wallets (id, provider, provider_id, public_key)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT DO NOTHING`
	_, err := pg.RawDB().ExecContext(ctx,
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
	ctx context.Context,
	tx *sqlx.Tx,
	ID string,
	userDepositDestination string,
	providerLinkingID uuid.UUID,
	anonymousAddress *uuid.UUID,
	userDepositAccountProvider string) error {

	var (
		statement string
		sqlErr    error
		r         sql.Result
	)

	id, err := uuid.FromString(ID)
	if err != nil {
		return err
	}

	if ok, err := pg.txHasDestination(ctx, tx, id); err != nil {
		return fmt.Errorf("error trying to lookup anonymous address: %w", err)
	} else if ok {
		statement = `
			UPDATE wallets
			SET
				provider_linking_id = $2,
				user_deposit_account_provider = $3
			WHERE id = $1;`
		r, sqlErr = tx.ExecContext(
			ctx,
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
		r, sqlErr = tx.ExecContext(
			ctx,
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

func txGetMaxLinkingSlots(ctx context.Context, tx *sqlx.Tx, providerLinkingID string) (int, error) {
	var (
		max int
	)
	statement := `
		select ($2 + count(1)) as max from linking_limit_adjust where provider_linking_id = $1
	`
	err := tx.Get(&max, statement, providerLinkingID, getEnvMaxCards())
	return max, err
}

func txGetUsedLinkingSlots(ctx context.Context, tx *sqlx.Tx, providerLinkingID string) (int, error) {
	var (
		used int
	)
	statement := `
		select count(1) as used from wallets where provider_linking_id = $1
	`
	err := tx.Get(&used, statement, providerLinkingID)
	return used, err
}

func bitFlyerRequestIDSpent(ctx context.Context, requestID string) bool {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}
	// get pg from context
	db, ok := ctx.Value(appctx.DatastoreCTXKey).(Datastore)
	if !ok {
		// if we cant check the db consider "spent"
		logger.Error().Msg("bitFlyerRequestIDSpent: unable to get datastore from context")
		return true
	}

	// attempt an insert into the spent bf request id table
	// if duplicate error, false
	if err := db.InsertBitFlyerRequestID(ctx, requestID); err != nil {
		// check error, consider "spent" if error
		logger.Error().Err(err).Msg("bitFlyerRequestIDSpent: database error attempting to insert")
		return true
	}
	// else not spent if successfully inserted
	return false
}

func getEnvMaxCards() int {
	if v, err := strconv.Atoi(os.Getenv("UPHOLD_WALLET_LINKING_LIMIT")); err == nil {
		return v
	}
	return 4
}

// LinkingInfo - a structure for wallet linking information
type LinkingInfo struct {
	WalletsLinked    int `json:"walletsLinked"`
	OpenLinkingSlots int `json:"openLinkingSlots"`
}

// GetLinkingLimitInfo - get some basic info about linking limit
func (pg *Postgres) GetLinkingLimitInfo(ctx context.Context, providerLinkingID string) (LinkingInfo, error) {
	var info = LinkingInfo{}

	tx, err := pg.RawDB().Beginx()
	if err != nil {
		return info, err
	}
	defer pg.RollbackTx(tx)

	maxLinkings, err := txGetMaxLinkingSlots(ctx, tx, providerLinkingID)
	if err != nil {
		return info, errorutils.Wrap(err, "error looking up max linkings for wallet")
	}

	usedLinkings, err := txGetUsedLinkingSlots(ctx, tx, providerLinkingID)
	if err != nil {
		return info, errorutils.Wrap(err, "error looking up used linkings for wallet")
	}

	info.WalletsLinked = usedLinkings
	info.OpenLinkingSlots = maxLinkings - usedLinkings

	return info, nil

}

// IncreaseLinkingLimit - increase the linking limit for the given walletID by one
func (pg *Postgres) IncreaseLinkingLimit(ctx context.Context, providerLinkingID uuid.UUID) error {
	statement := `INSERT INTO linking_limit_adjust (provider_linking_id) VALUES ($1)`
	_, err := pg.RawDB().ExecContext(ctx, statement, providerLinkingID)
	if err != nil {
		return err
	}
	return nil
}

// LinkWallet links a wallet together
func (pg *Postgres) LinkWallet(ctx context.Context, ID string, userDepositDestination string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID, depositProvider string) error {
	tx, err := pg.RawDB().Beginx()
	if err != nil {
		return err
	}
	defer pg.RollbackTx(tx)

	maxLinkings, err := txGetMaxLinkingSlots(ctx, tx, providerLinkingID.String())
	if err != nil {
		return errorutils.Wrap(err, "error looking up max linkings for wallet")
	}

	usedLinkings, err := txGetUsedLinkingSlots(ctx, tx, providerLinkingID.String())
	if err != nil {
		return errorutils.Wrap(err, "error looking up used linkings for wallet")
	}

	if usedLinkings >= maxLinkings {
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

	err = pg.TxLinkWalletInfo(ctx, tx, ID, userDepositDestination, providerLinkingID, anonymousAddress, depositProvider)
	if err != nil {
		return errorutils.Wrap(err, "unable to set an anonymous address")
	}

	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

// CustodianLink - representation of wallet_custodian record
type CustodianLink struct {
	ID                 *uuid.UUID `json:"id" db:"id" valid:"uuidv4"`
	WalletID           *uuid.UUID `json:"wallet_id" db:"wallet_id" valid:"uuidv4"`
	Custodian          string     `json:"custodian" db:"custodian" valid:"in(uphold,brave,gemini,bitflyer)"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at" valid:"-"`
	LinkedAt           time.Time  `json:"linked_at" db:"linked_at" valid:"-"`
	DisconnectedAt     time.Time  `json:"disconnected_at" db:"disconnected_at" valid:"-"`
	DepositDestination string     `json:"deposit_destination" db:"deposit_destination" valid:"-"`
	LinkingID          *uuid.UUID `json:"linking_id" db:"linking_id" valid:"uuid"`
}

// GetWalletIDString - get string version of the WalletID
func (cl *CustodianLink) GetWalletIDString() string {
	if cl.WalletID != nil {
		return cl.WalletID.String()
	}
	return ""
}

// GetLinkingIDString - get string version of the LinkingID
func (cl *CustodianLink) GetLinkingIDString() string {
	if cl.LinkingID != nil {
		return cl.LinkingID.String()
	}
	return ""
}

// GetCustodianLinkCount - get the wallet custodian link count across all wallets
func (pg *Postgres) GetCustodianLinkCount(ctx context.Context, linkingID uuid.UUID) (int, int, error) {
	// the count of linked wallets
	used := 0
	max := 0
	var err error

	// create a sublogger
	sublogger := logger(ctx).With().
		Str("linking_id", linkingID.String()).
		Logger()

	sublogger.Debug().
		Msg("starting GetCustodianLinkCount")

	// get tx
	tx, noContextTx := ctx.Value(appctx.DatabaseTransactionCTXKey).(*sqlx.Tx)
	if !noContextTx {
		sublogger.Debug().
			Msg("no tx in context")
		tx, err = createTx(ctx, pg)
		if err != nil || tx == nil {
			sublogger.Error().Err(err).
				Msg("error creating tx")
			return 0, 0, fmt.Errorf("failed to create tx for GetCustodianLinkCount: %w", err)
		}
		// add tx to ctx for future
		defer pg.RollbackTx(tx)
	}

	// get used
	stmt := `
		select count(distinct(wallet_id)) as used from wallet_custodian where linking_id = $1
	`
	err = tx.Get(&used, stmt, linkingID)
	if err != nil {
		sublogger.Error().Err(err).
			Msg("failed to get CustodianLinkCount from DB")
		return 0, 0, fmt.Errorf("failed to get CustodianLinkCount from DB: %w", err)
	}
	// get max
	stmt = `
		select ($2 + count(1)) as max from linking_limit_adjust where provider_linking_id = $1
	`
	err = tx.Get(&max, stmt, linkingID, getEnvMaxCards())
	if err != nil {
		sublogger.Error().Err(err).
			Msg("failed to get CustodianLinkCount from DB")
		return 0, 0, fmt.Errorf("failed to get CustodianLinkCount from DB: %w", err)
	}

	if !noContextTx {
		// commit
		err = tx.Commit()
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to commit transaction")
			return 0, 0, fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	return used, max, nil
}

// GetCustodianLinkByID - get the wallet custodian record by id
func (pg *Postgres) GetCustodianLinkByID(ctx context.Context, ID uuid.UUID) (*CustodianLink, error) {
	var (
		cl  = new(CustodianLink)
		err error
	)
	// create a sublogger
	sublogger := logger(ctx).With().
		Str("custodian_link_id", ID.String()).
		Logger()

	sublogger.Debug().
		Msg("starting GetCustodianLinkByID")

	// get tx
	tx, noContextTx := ctx.Value(appctx.DatabaseTransactionCTXKey).(*sqlx.Tx)
	if !noContextTx {
		sublogger.Debug().
			Msg("no tx in context")
		tx, err = createTx(ctx, pg)
		if err != nil || tx == nil {
			sublogger.Error().Err(err).
				Msg("error creating tx")
			return nil, fmt.Errorf("failed to create tx for GetCustodianLinkByID: %w", err)
		}
		// add tx to ctx for future
		defer pg.RollbackTx(tx)
	}

	// query
	stmt := `
		select
			id, wallet_id, custodian, created_at, linked_at, disconnected_at, deposit_destination, linking_id
		from
			wallet_custodian
		where
			id = $1
	`
	err = tx.Get(cl, stmt, ID)
	if err != nil {
		sublogger.Error().Err(err).
			Msg("failed to get CustodianLink from DB")
		return nil, fmt.Errorf("failed to get CustodianLink from DB: %w", err)
	}

	if !noContextTx {
		// commit
		err = tx.Commit()
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to commit transaction")
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}
	}
	return cl, nil
}

// GetCustodianLinkByWalletID - get the wallet custodian record by id
func (pg *Postgres) GetCustodianLinkByWalletID(ctx context.Context, ID uuid.UUID) (*CustodianLink, error) {
	var (
		cl  = new(CustodianLink)
		err error
	)
	// create a sublogger
	sublogger := logger(ctx).With().
		Str("wallet_id", ID.String()).
		Logger()

	sublogger.Debug().
		Msg("starting GetCustodianLinkByWalletID")

	// get tx
	tx, noContextTx := ctx.Value(appctx.DatabaseTransactionCTXKey).(*sqlx.Tx)
	if !noContextTx {
		sublogger.Debug().
			Msg("no tx in context")
		tx, err = createTx(ctx, pg)
		if err != nil || tx == nil {
			sublogger.Error().Err(err).
				Msg("error creating tx")
			return nil, fmt.Errorf("failed to create tx for GetCustodianLinkByID: %w", err)
		}
		// add tx to ctx for future
		defer pg.RollbackTx(tx)
	}
	// query
	stmt := `
		select
			wc.id, wc.wallet_id, wc.custodian, wc.created_at, wc.linked_at,
			wc.disconnected_at, wc.deposit_destination, wc.linking_id
		from
			wallet_custodian wc
		join
			wallets w on (w.wallet_custodian_id = wc.id)
		where
			w.id = $1
	`
	err = tx.Get(cl, stmt, ID)
	if err != nil {
		sublogger.Error().Err(err).
			Msg("failed to get CustodianLink from DB")
		return nil, fmt.Errorf("failed to get CustodianLink from DB: %w", err)
	}

	if !noContextTx {
		// commit
		err = tx.Commit()
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to commit transaction")
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}
	}
	return cl, nil
}

// DisconnectCustodialWallet - disconnect the wallet's custodial id
func (pg *Postgres) DisconnectCustodialWallet(ctx context.Context, walletID uuid.UUID) error {
	// create a sublogger
	sublogger := logger(ctx).With().
		Str("wallet_id", walletID.String()).
		Logger()

	sublogger.Debug().
		Msg("disconnecting custodial wallet")

	// create tx
	tx, err := createTx(ctx, pg)
	if err != nil || tx == nil {
		sublogger.Error().Err(err).
			Msg("error creating tx for wallet linking")
		return fmt.Errorf("failed to create tx for wallet linking: %w", err)
	}
	// add tx to ctx for future
	ctx = context.WithValue(ctx, appctx.DatabaseTransactionCTXKey, tx)
	defer pg.RollbackTx(tx)

	// TODO: if this wallet has no wallet_custodian_id set, but
	// does have provider_linking_id and user_deposit_account_provider and user_deposit_destination
	// then we need to take the following actions:
	// 1.) InsertCustodianLink with the aforementioned information
	// 2.) Update the wallet with the link to the inserted custodian link
	// 3.) Remove the provider_linking_id, user_deposit_account_provider and user_deposit_destination
	//     from the wallet table
	// 4.) Continue processing this method
	// This will ensure that the record is in the right state prior to the disconnect process.
	if ok, err := pg.CustodianLinkMigrated(ctx, walletID); err != nil {
		// error checking if custodian link was migrated
	} else if !ok {
		// this wallet has not yet been migrated to the CustodianLink model
		if err := pg.MigrateCustodianLink(ctx, walletID); err != nil {

		}
	}

	// sql query to perform
	stmt := `
		update 
			wallet_custodian as wc
		set disconnected_at=now()
		from wallets as w
		where 
			w.wallet_custodian_id=wc.id
			and w.id=$1
	`
	// perform query to set disconnected time on wallet custodian
	if r, err := tx.ExecContext(
		ctx,
		stmt,
		walletID,
	); err != nil {
		sublogger.Error().Err(err).Msg("failed to update wallet_custodian_id for wallet")
		return err
	} else if r != nil {
		count, _ := r.RowsAffected()
		if count < 1 {
			sublogger.Error().Msg("at least one record should be updated for disconnecting a verified wallet")
			return errors.New("should have updated at least one wallet with disconnected custodial")
		}
	}

	// sql query to perform unlinking
	stmt = `
		update wallets set wallet_custodian_id=null where id=$1
	`
	// perform query
	if r, err := tx.ExecContext(
		ctx,
		stmt,
		walletID,
	); err != nil {
		sublogger.Error().Err(err).Msg("failed to update wallet_custodian_id for wallet")
		return err
	} else if r != nil {
		count, _ := r.RowsAffected()
		if count < 1 {
			sublogger.Error().Msg("at least one record should be updated for disconnecting a verified wallet")
			return errors.New("should have updated at least one wallet with disconnected custodial")
		}
	}

	// commit
	err = tx.Commit()
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to commit transaction")
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// done
	return nil
}

// InsertCustodianLink - create a record of a custodian wallet
func (pg *Postgres) InsertCustodianLink(ctx context.Context, cl *CustodianLink) error {
	var err error
	// create a sublogger
	sublogger := logger(ctx).With().
		Str("wallet_id", cl.GetWalletIDString()).
		Str("custodian", cl.Custodian).
		Str("linking_id", cl.GetLinkingIDString()).
		Logger()

	sublogger.Debug().
		Msg("creating linking of wallet custodian")

	// get tx
	tx, noContextTx := ctx.Value(appctx.DatabaseTransactionCTXKey).(*sqlx.Tx)
	if !noContextTx {
		tx, err = createTx(ctx, pg)
		if err != nil || tx == nil {
			sublogger.Error().Err(err).
				Msg("error creating tx for wallet linking")
			return fmt.Errorf("failed to create tx for wallet linking: %w", err)
		}
		// add tx to ctx for future
		defer pg.RollbackTx(tx)
	}

	// get the count
	used, max, err := pg.GetCustodianLinkCount(ctx, *cl.LinkingID)
	if err != nil {
		sublogger.Error().Err(err).
			Msg("failed to insert wallet_custodian due to db err checking linking limits")
		return fmt.Errorf("failed to insert wallet custodian record due to db err checking linking limits: %w", err)
	}

	// check for linking limit
	if used >= max {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTags(map[string]string{
				"wallet_id":  cl.WalletID.String(),
				"linking_id": cl.LinkingID.String(),
			})
			tooManyCardsCounter.Inc()
		})
		return ErrTooManyCardsLinked
	}

	// check the linking limit does not exceed what is appropriate
	if err != nil {
		sublogger.Error().Err(err).
			Msg("failed to insert wallet_custodian due to db err checking linking limits")
		return fmt.Errorf("failed to insert wallet custodian record due to db err checking linking limits: %w", err)
	}

	// TODO: check linking limits here
	stmt := `
		insert into wallet_custodian (
			wallet_id, custodian, deposit_destination, linking_id
		) values (
			$1, $2, $3, $4
		) returning id, created_at, linked_at
	`

	if cl.ID == nil {
		cl.ID = new(uuid.UUID)
	}

	err = tx.Get(cl, stmt, cl.WalletID, cl.Custodian, cl.DepositDestination, cl.LinkingID)
	if err != nil {
		sublogger.Error().Err(err).
			Msg("failed to insert wallet_custodian")
		return fmt.Errorf("failed to insert wallet custodian record: %w", err)
	}

	if !noContextTx {
		// commit
		err = tx.Commit()
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to commit transaction")
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	}
	return nil
}

// helper to make logger easier
func logger(ctx context.Context) *zerolog.Logger {
	// get logger
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		_, logger = logging.SetupLogger(ctx)
	}
	return logger
}

// ConnectCustodialWallet - connect the wallet's custodial verified wallet.
func (pg *Postgres) ConnectCustodialWallet(ctx context.Context, cl CustodianLink) error {
	var err error
	// create a sublogger
	sublogger := logger(ctx).With().
		Str("wallet_id", cl.GetWalletIDString()).
		Str("custodian", cl.Custodian).
		Str("linking_id", cl.GetLinkingIDString()).
		Logger()

	sublogger.Debug().
		Msg("creating linking of wallet custodian")

	// create tx
	tx, err := createTx(ctx, pg)
	if err != nil || tx == nil {
		sublogger.Error().Err(err).
			Msg("error creating tx for wallet linking")
		return fmt.Errorf("failed to create tx for wallet linking: %w", err)
	}
	// add tx to ctx for future
	ctx = context.WithValue(ctx, appctx.DatabaseTransactionCTXKey, tx)
	defer pg.RollbackTx(tx)

	// if cl is not defined, this must be a new wallet linking attempt
	if cl.ID == nil {
		// insert wallet custodian (which will update with an ID)
		err := pg.InsertCustodianLink(ctx, &cl)
		if err != nil {
			sublogger.Error().Err(err).
				Msg("error inserting wallet linking")
			return fmt.Errorf("failed to insert the wallet linking: %w", err)
		}
	}
	sublogger.Debug().Msg("inserted custodian link")
	// link the new wallet custodian to the wallet record in db
	stmt := `
		update wallets set wallet_custodian_id=$2 where id=$1
	`
	// perform query
	if r, err := tx.ExecContext(
		ctx,
		stmt,
		cl.WalletID,
		cl.ID,
	); err != nil {
		sublogger.Error().Err(err).
			Str("wallet_custodian_id", cl.ID.String()).
			Msg("failed to update wallet_custodian_id for wallet")
		return fmt.Errorf("error updating wallet with new wallet_custodian: %w", err)
	} else if r != nil {
		count, _ := r.RowsAffected()
		if count < 1 {
			sublogger.Error().Msg("at least one record should be updated for disconnecting a verified wallet")
			return errors.New("should have updated at least one wallet with disconnected custodial")
		}
	}

	// commit
	err = tx.Commit()
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to commit transaction")
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// helper to create a tx
func createTx(ctx context.Context, pg *Postgres) (tx *sqlx.Tx, err error) {
	// get or create tx
	logger(ctx).Debug().
		Msg("no transaction on context")
	// no tx, create one and rollback on defer, adding to ctx
	tx, err = pg.RawDB().Beginx()
	if err != nil {
		logger(ctx).Error().Err(err).
			Msg("error creating transaction")
		return tx, fmt.Errorf("failed to create transaction: %w", err)
	}
	return tx, nil
}

package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/reputation"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	timeutils "github.com/brave-intl/bat-go/utils/time"
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
	metricTxLockGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "pg_tx_advisory_lock_gauge",
			Help:        "Monitors number of tx advisory locks",
			ConstLabels: prometheus.Labels{"service": "wallet"},
		})
	tooManyCardsCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:        "too_many_linked_cards",
			Help:        "A counter for too many linked cards",
			ConstLabels: prometheus.Labels{"service": "wallet"},
		})
)

func init() {
	prometheus.MustRegister(tooManyCardsCounter)
	prometheus.MustRegister(metricTxLockGauge)
}

// Datastore holds the interface for the wallet datastore
type Datastore interface {
	grantserver.Datastore
	LinkWallet(ctx context.Context, ID string, providerID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID, depositProvider string) error
	IncreaseLinkingLimit(ctx context.Context, providerLinkingID uuid.UUID) error
	UnlinkWallet(ctx context.Context, walletID uuid.UUID, custodian string) error
	GetLinkingLimitInfo(ctx context.Context, providerLinkingID string) (map[string]LinkingInfo, error)
	// GetLinkingsByProviderLinkingID gets the wallet linking info by provider linking id
	GetLinkingsByProviderLinkingID(ctx context.Context, providerLinkingID uuid.UUID) ([]LinkingMetadata, error)
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
	ConnectCustodialWallet(ctx context.Context, cl *CustodianLink, depositDest string) error
	// DisconnectCustodialWallet - disconnect the wallet's custodial id
	DisconnectCustodialWallet(ctx context.Context, walletID uuid.UUID) error
	// GetCustodianLinkByWalletID - get the custodian link by ID
	GetCustodianLinkByWalletID(ctx context.Context, ID uuid.UUID) (*CustodianLink, error)
	// GetCustodianLinkCount - get the wallet custodian link count across all wallets
	GetCustodianLinkCount(ctx context.Context, linkingID uuid.UUID, custodian string) (int, int, error)
}

// ReadOnlyDatastore includes all database methods that can be made with a read only db connection
type ReadOnlyDatastore interface {
	grantserver.Datastore
	// GetLinkingsByProviderLinkingID gets the wallet linking info by provider linking id
	GetLinkingsByProviderLinkingID(ctx context.Context, providerLinkingID uuid.UUID) ([]LinkingMetadata, error)
	// GetByProviderLinkingID gets a wallet by provider linking id
	GetByProviderLinkingID(ctx context.Context, providerLinkingID uuid.UUID) (*[]walletutils.Info, error)
	// GetWallet by ID
	GetWallet(ctx context.Context, ID uuid.UUID) (*walletutils.Info, error)
	// GetWalletByPublicKey
	GetWalletByPublicKey(context.Context, string) (*walletutils.Info, error)
	// GetCustodianLinkByWalletID - get the current custodian link by wallet id
	GetCustodianLinkByWalletID(ctx context.Context, ID uuid.UUID) (*CustodianLink, error)
	// GetCustodianLinkCount - get the wallet custodian link count across all wallets
	GetCustodianLinkCount(ctx context.Context, linkingID uuid.UUID, custodian string) (int, int, error)
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

// GetLinkingsByProviderLinkingID gets wallet linkings by a provider address
func (pg *Postgres) GetLinkingsByProviderLinkingID(ctx context.Context, providerLinkingID uuid.UUID) ([]LinkingMetadata, error) {
	statement := `
	select
		wallet_id, disconnected_at, created_at, linked_at, unlinked_at,
		(disconnected_at is null and unlinked_at is null) as active
	from
		wallet_custodian
	WHERE linking_id = $1
	`
	var linkings []LinkingMetadata
	err := pg.RawDB().SelectContext(ctx, &linkings, statement, providerLinkingID)
	return linkings, err
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

type custodianLinkingID struct {
	Custodian string     `db:"custodian"`
	LinkingID *uuid.UUID `db:"linking_id"`
}

func txGetCustodianLinkingIDs(ctx context.Context, tx *sqlx.Tx, providerLinkingID string) (map[string]string, error) {
	var (
		custodianLinkingIDs = []custodianLinkingID{}
		resp                = map[string]string{}
	)
	statement := `
		select wc1.custodian, wc1.linking_id from wallet_custodian wc1 join wallet_custodian wc2 on
		(wc1.wallet_id=wc2.wallet_id) where wc2.linking_id = $1 and wc1.unlinked_at is null and wc2.unlinked_at is null
	`
	err := tx.Select(&custodianLinkingIDs, statement, providerLinkingID)
	if err != nil {
		return nil, fmt.Errorf("failed to associate linking id to custodians: %w", err)
	}

	for _, v := range custodianLinkingIDs {
		resp[v.Custodian] = v.LinkingID.String()
	}

	return resp, nil
}

func txGetMaxLinkingSlots(ctx context.Context, tx *sqlx.Tx, custodian, providerLinkingID string) (int, error) {
	var (
		max int
	)
	statement := `
		select ($2 + count(1)) as max from linking_limit_adjust where provider_linking_id = $1
	`
	err := tx.Get(&max, statement, providerLinkingID, getEnvMaxCards(custodian))
	return max, err
}

func txGetUsedLinkingSlots(ctx context.Context, tx *sqlx.Tx, providerLinkingID string) (int, error) {
	var (
		used int
	)
	// we need to exclude `this` wallet from the used computation in the event we are attempting
	// to re-link the 4th slot
	statement := `
		select count(distinct(wallet_id)) as used from wallet_custodian where linking_id = $1 and unlinked_at is null
	`
	err := tx.Get(&used, statement, providerLinkingID)
	return used, err
}

func bitFlyerRequestIDSpent(ctx context.Context, requestID string) bool {
	logger := logging.Logger(ctx, "wallet.bitFlyerRequestIDSpent")
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

func getEnvMaxCards(custodian string) int {
	switch custodian {
	case "uphold":
		if v, err := strconv.Atoi(os.Getenv("UPHOLD_WALLET_LINKING_LIMIT")); err == nil {
			return v
		}
	case "bitflyer":
		if v, err := strconv.Atoi(os.Getenv("BITFLYER_WALLET_LINKING_LIMIT")); err == nil {
			return v
		}
	case "gemini":
		if v, err := strconv.Atoi(os.Getenv("GEMINI_WALLET_LINKING_LIMIT")); err == nil {
			return v
		}
	}
	return 4
}

// LinkingMetadata - show more details in linking info about the linkages
type LinkingMetadata struct {
	WalletID       uuid.UUID  `json:"id" db:"wallet_id"`
	DisconnectedAt *time.Time `json:"disconnectedAt,omitempty" db:"disconnected_at"`
	LastLinkedAt   *time.Time `json:"lastLinkedAt,omitempty" db:"linked_at"`
	FirstLinkedAt  *time.Time `json:"firstLinkedAt,omitempty" db:"created_at"`
	UnLinkedAt     *time.Time `json:"unlinkedAt,omitempty" db:"unlinked_at"`
	Active         bool       `json:"active" db:"active"`
}

// LinkingInfo - a structure for wallet linking information
type LinkingInfo struct {
	LinkingID              *uuid.UUID        `json:"-"`
	NextAvailableUnlinking *time.Time        `json:"nextAvailableUnlinking,omitempty"`
	WalletsLinked          int               `json:"walletsLinked"`
	OpenLinkingSlots       int               `json:"openLinkingSlots"`
	OtherWalletsLinked     []LinkingMetadata `json:"otherWalletsLinked,omitempty"`
}

// GetLinkingLimitInfo - get some basic info about linking limit
func (pg *Postgres) GetLinkingLimitInfo(ctx context.Context, providerLinkingID string) (map[string]LinkingInfo, error) {
	var infos = map[string]LinkingInfo{}

	// get tx
	_, tx, rollback, commit, err := getTx(ctx, pg)
	if err != nil {
		return nil, fmt.Errorf("failed to create db transaction GetLinkingLimitInfo: %w", err)
	}
	// will rollback if tx created at this scope
	defer rollback()

	// find all custodians that have been linked to this wallet based on providerLinkingID
	custodianLinkingIDs, err := txGetCustodianLinkingIDs(ctx, tx, providerLinkingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get custodian linking ids: %w", err)
	}

	// for each custodian linking id found, get the max/used
	for custodian, linkingID := range custodianLinkingIDs {
		maxLinkings, err := txGetMaxLinkingSlots(ctx, tx, custodian, linkingID)
		if err != nil {
			return nil, errorutils.Wrap(err, "error looking up max linkings for wallet")
		}

		usedLinkings, err := txGetUsedLinkingSlots(ctx, tx, linkingID)
		if err != nil {
			return nil, errorutils.Wrap(err, "error looking up used linkings for wallet")
		}

		// convert linking id to uuid
		lID, err := uuid.FromString(linkingID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse linking id: %w", err)
		}

		// lookup other linked wallets
		linkings, err := pg.GetLinkingsByProviderLinkingID(ctx, lID)
		if err != nil {
			return nil, fmt.Errorf("failed to get other wallets by linking id: %w", err)
		}

		// get the next available unlinking time
		// lower bound based
		lbDur, ok := ctx.Value(appctx.NoUnlinkPriorToDurationCTXKey).(string)
		if !ok {
			return nil, fmt.Errorf("misconfigured service, no unlink prior to duration configured")
		}

		d, err := timeutils.ParseDuration(lbDur)
		if err != nil {
			return nil, fmt.Errorf("misconfigured service, invalid no unlink prior to duration configured")
		}

		// get the latest unlinking
		stmt := `
			select
				max(unlinked_at) as last_unlinking
			from
				wallet_custodian
			where
				linking_id = $1 and unlinked_at is not null
		`
		var last *time.Time
		err = tx.Get(&last, stmt, lID)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("failed to get max time: %w", err)
		}

		var nextUnlink time.Time

		if last != nil {
			notBefore, err := d.FromNow()
			if err != nil {
				return nil, fmt.Errorf("unable to get not before time from duration: %w", err)
			}
			if (*notBefore).Before(*last) {
				// last - notBefore is the duration we need to cool off
				// now + ( last - notBefore )
				nextUnlink = time.Now().Add(last.Sub(*notBefore))
			}
		}

		// add to result
		infos[custodian] = LinkingInfo{
			NextAvailableUnlinking: &nextUnlink,
			LinkingID:              &lID,
			WalletsLinked:          usedLinkings,
			OpenLinkingSlots:       maxLinkings - usedLinkings,
			OtherWalletsLinked:     linkings,
		}
	}

	// if the tx was created in this scope we will commit here
	if err := commit(); err != nil {
		return nil, fmt.Errorf("failed to commit GetLinkingLimitInfo transaction: %w", err)
	}

	return infos, nil
}

// ErrUnlinkingsExceeded - the number of custodian wallet unlinkings attempts have exceeded
var ErrUnlinkingsExceeded = errors.New("custodian unlinking limit reached")

// UnlinkWallet - unlink the wallet from the custodian completely
func (pg *Postgres) UnlinkWallet(ctx context.Context, walletID uuid.UUID, custodian string) error {
	// get tx
	ctx, tx, rollback, commit, err := getTx(ctx, pg)
	if err != nil {
		return fmt.Errorf("failed to create db transaction UnlinkWallet: %w", err)
	}
	// will rollback if tx created at this scope
	defer rollback()

	// lower bound based
	lbDur, ok := ctx.Value(appctx.NoUnlinkPriorToDurationCTXKey).(string)
	if !ok {
		return fmt.Errorf("misconfigured service, no unlink prior to duration configured")
	}

	d, err := timeutils.ParseDuration(lbDur)
	if err != nil {
		return fmt.Errorf("misconfigured service, invalid no unlink prior to duration configured")
	}

	notBefore, err := d.FromNow()
	if err != nil {
		return fmt.Errorf("unable to get not before time from duration: %w", err)
	}

	// validate that no other linkages were unlinked in the duration specified
	stmt := `
		select
			count(wc2.*)
		from
			wallet_custodian wc1 join wallet_custodian wc2 on wc1.linking_id=wc2.linking_id
		where
			wc1.wallet_id=$1 and wc1.custodian=$2 and wc2.unlinked_at>$3
	`
	var count int
	err = tx.Get(&count, stmt, walletID, custodian, notBefore)
	if err != nil {
		return err
	}

	if count > 0 {
		return ErrUnlinkingsExceeded
	}

	statement := `update wallet_custodian set unlinked_at=now() where wallet_id = $1 and custodian = $2`
	_, err = tx.ExecContext(ctx, statement, walletID, custodian)
	if err != nil {
		return err
	}

	// remove the user_deposit_destination, user_account_deposit_provider from the wallets table

	statement = `update wallets set user_deposit_destination='',user_deposit_account_provider=null where id = $1`

	_, err = tx.ExecContext(ctx, statement, walletID)
	if err != nil {
		return err
	}

	if err := commit(); err != nil {
		return fmt.Errorf("failed to commit tx: %w", err)
	}
	return nil
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

var (
	// ErrUnusualActivity - error for wallets with unusual activity
	ErrUnusualActivity = errors.New("unusual activity")
)

// LinkWallet links a wallet together
func (pg *Postgres) LinkWallet(ctx context.Context, ID string, userDepositDestination string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID, depositProvider string) error {

	sublogger := logger(ctx).With().Str("wallet_id", ID).Logger()

	// rep check
	if repClient, ok := ctx.Value(appctx.ReputationClientCTXKey).(reputation.Client); ok {
		walletID, err := uuid.FromString(ID)
		if err != nil {
			sublogger.Warn().Err(err).Msg("invalid wallet id")
			return fmt.Errorf("invalid wallet id, not uuid: %w", err)
		}
		// we have a client, check the value for ID
		reputable, err := repClient.IsWalletAdsReputable(ctx, walletID, "")
		if err != nil {
			sublogger.Warn().Err(err).Msg("failed to check reputation")
			return fmt.Errorf("failed to check wallet rep: %w", err)
		}

		if !reputable {
			sublogger.Info().Msg("wallet linking attempt failed - unusual activity")
			return ErrUnusualActivity
		}
	}

	ctx, tx, rollback, commit, err := getTx(ctx, pg)
	if err != nil {
		sublogger.Error().Err(err).Msg("error getting tx")
		return fmt.Errorf("error getting tx: %w", err)
	}
	defer func() {
		metricTxLockGauge.Dec()
		rollback()
	}()

	metricTxLockGauge.Inc()
	err = waitAndLockTx(ctx, tx, providerLinkingID)
	if err != nil {
		sublogger.Error().Err(err).Msg("error acquiring tx lock")
		return fmt.Errorf("error acquiring tx lock: %w", err)
	}

	id, err := uuid.FromString(ID)
	if err != nil {
		return errorutils.Wrap(err, "error invalid id")
	}

	// connect custodian link (does the link limit checking in insert)
	if err = pg.ConnectCustodialWallet(ctx, &CustodianLink{
		WalletID:  &id,
		Custodian: depositProvider,
		LinkingID: &providerLinkingID,
	}, userDepositDestination); err != nil {
		sublogger.Error().Err(err).
			Msg("error connect custodian wallet")
		return fmt.Errorf("error connect custodian wallet: %w", err)
	}

	err = commit()
	if err != nil {
		sublogger.Error().Err(err).
			Msg("error committing tx")
		return fmt.Errorf("error committing tx: %w", err)
	}

	return nil
}

// CustodianLink - representation of wallet_custodian record
type CustodianLink struct {
	WalletID           *uuid.UUID `json:"wallet_id" db:"wallet_id" valid:"uuidv4"`
	Custodian          string     `json:"custodian" db:"custodian" valid:"in(uphold,brave,gemini,bitflyer)"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at" valid:"-"`
	LinkedAt           time.Time  `json:"linked_at" db:"linked_at" valid:"-"`
	DisconnectedAt     *time.Time `json:"disconnected_at" db:"disconnected_at" valid:"-"`
	DepositDestination string     `json:"deposit_destination" db:"deposit_destination" valid:"-"`
	LinkingID          *uuid.UUID `json:"linking_id" db:"linking_id" valid:"uuid"`
	UnlinkedAt         *time.Time `json:"unlinked_at" db:"unlinked_at" valid:"-"`
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
func (pg *Postgres) GetCustodianLinkCount(ctx context.Context, linkingID uuid.UUID, custodian string) (int, int, error) {
	// the count of linked wallets
	var err error

	// create a sublogger
	sublogger := logger(ctx).With().
		Str("linking_id", linkingID.String()).
		Logger()

	sublogger.Debug().
		Msg("starting GetCustodianLinkCount")

	// get tx
	ctx, _, rollback, commit, err := getTx(ctx, pg)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create db transaction GetCustodianLinkByWalletID: %w", err)
	}
	// will rollback if tx created at this scope
	defer rollback()

	li, err := pg.GetLinkingLimitInfo(ctx, linkingID.String())
	if err != nil {
		sublogger.Error().Err(err).
			Msg("failed to get CustodianLinkCount from DB")
		return 0, 0, fmt.Errorf("failed to get CustodianLinkCount from DB: %w", err)
	}

	// if the tx was created in this scope we will commit here
	if err := commit(); err != nil {
		return 0, 0, fmt.Errorf("failed to commit GetCustodianByWalletID transaction: %w", err)
	}

	for _, linkingInfo := range li {
		// find the right linking id
		if linkingInfo.LinkingID.String() == linkingID.String() {
			// max is wallets linked + open slots
			return linkingInfo.WalletsLinked, linkingInfo.WalletsLinked + linkingInfo.OpenLinkingSlots, nil
		}
	}
	// wallets linked/ open linking slots not found
	// this is the case where there is no prior linkages
	// 0 linked, get max from environment
	return 0, getEnvMaxCards(custodian), nil

}

func rollbackFn(ctx context.Context, pg *Postgres, tx *sqlx.Tx) func() {
	return func() {
		logger(ctx).Debug().Msg("rolling back transaction")
		pg.RollbackTx(tx)
	}
}

func commitFn(ctx context.Context, tx *sqlx.Tx) func() error {
	return func() error {
		logger(ctx).Debug().Msg("committing transaction")
		if err := tx.Commit(); err != nil {
			logger(ctx).Error().Err(err).Msg("failed to commit transaction")
			return err
		}
		return nil
	}
}

// getTx will get or create a tx on the context, if created hands back rollback and commit functions
func getTx(ctx context.Context, pg *Postgres) (context.Context, *sqlx.Tx, func(), func() error, error) {
	// create a sublogger
	sublogger := logger(ctx)
	sublogger.Debug().Msg("getting tx from context")
	// get tx
	tx, noContextTx := ctx.Value(appctx.DatabaseTransactionCTXKey).(*sqlx.Tx)
	if !noContextTx {
		sublogger.Debug().Msg("no tx in context")
		tx, err := createTx(ctx, pg)
		if err != nil || tx == nil {
			sublogger.Error().Err(err).Msg("error creating tx")
			return ctx, nil, func() {}, func() error { return nil }, fmt.Errorf("failed to create tx: %w", err)
		}
		ctx = context.WithValue(ctx, appctx.DatabaseTransactionCTXKey, tx)
		return ctx, tx, rollbackFn(ctx, pg, tx), commitFn(ctx, tx), nil
	}
	return ctx, tx, func() {}, func() error { return nil }, nil
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
	_, tx, rollback, commit, err := getTx(ctx, pg)
	if err != nil {
		return nil, fmt.Errorf("failed to create db transaction GetCustodianLinkByWalletID: %w", err)
	}
	// will rollback if tx created at this scope
	defer rollback()

	// query
	stmt := `
		select
			wc.wallet_id, wc.custodian, wc.linking_id,
			wc.created_at, wc.disconnected_at, wc.linked_at
		from
			wallet_custodian wc
		where
			wc.wallet_id = $1 and
			wc.disconnected_at is null and
			wc.unlinked_at is null
	`
	err = tx.Get(cl, stmt, ID)
	if err != nil {
		sublogger.Error().Err(err).
			Msg("failed to get CustodianLink from DB")
		return nil, fmt.Errorf("failed to get CustodianLink from DB: %w", err)
	}

	// if the tx was created in this scope we will commit here
	if err := commit(); err != nil {
		return nil, fmt.Errorf("failed to commit GetCustodianByWalletID transaction: %w", err)
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

	// get tx
	ctx, tx, rollback, commit, err := getTx(ctx, pg)
	if err != nil {
		return fmt.Errorf("failed to create db transaction DisconnectCustodialWallet: %w", err)
	}
	// will rollback if tx created at this scope
	defer rollback()

	// sql query to perform unlinking
	stmt := `
		update
			wallets
		set
			user_deposit_destination=''
		where
			id=$1
	`
	// perform query
	if _, err := tx.ExecContext(
		ctx,
		stmt,
		walletID,
	); err != nil {
		sublogger.Error().Err(err).Msg("failed to update wallet_custodian_id for wallet")
		return err
	}

	// set disconnected on the custodian link
	stmt = `
		update
			wallet_custodian
		set
			disconnected_at=now()
		where
			wallet_id=$1 and
			disconnected_at is null and
			unlinked_at is null
	`
	// perform query
	if _, err := tx.ExecContext(
		ctx,
		stmt,
		walletID,
	); err != nil {
		sublogger.Error().Err(err).Msg("failed to update wallet_custodian_id for wallet")
		return err
	}

	// if the tx was created in this scope we will commit here
	if err := commit(); err != nil {
		return fmt.Errorf("failed to commit DisconnectCustodialWallet transaction: %w", err)
	}
	// done
	return nil
}

// ConnectCustodialWallet - create a record of a custodian wallet
func (pg *Postgres) ConnectCustodialWallet(ctx context.Context, cl *CustodianLink, depositDest string) error {
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
	ctx, tx, rollback, commit, err := getTx(ctx, pg)
	if err != nil {
		return fmt.Errorf("failed to create db transaction ConnectCustodialWallet: %w", err)
	}
	// will rollback if tx created at this scope
	defer rollback()

	var existingLinkingID uuid.UUID
	// get the custodial provider's linking id from db
	stmt := `
		select linking_id from wallet_custodian
		where wallet_id=$1 and custodian=$2 and
		unlinked_at is null
	`
	err = tx.Get(&existingLinkingID, stmt, cl.WalletID, cl.Custodian)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		sublogger.Error().Err(err).
			Msg("failed to get linking id from wallet_custodian")
		return fmt.Errorf("failed to get linking id from custodian record: %w", err)
	}

	if !uuid.Equal(existingLinkingID, *new(uuid.UUID)) {
		// check if the member matches the associated member
		if !uuid.Equal(*cl.LinkingID, existingLinkingID) {
			return handlers.WrapError(errors.New("wallets do not match"), "mismatched provider accounts", http.StatusForbidden)
		}
	} else {
		// if the existingLinkingID is null then we need to check the linking limits

		// get the count
		used, max, err := pg.GetCustodianLinkCount(ctx, *cl.LinkingID, cl.Custodian)
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
	}

	stmt = `
		insert into wallet_custodian (
			wallet_id, custodian, linking_id
		) values (
			$1, $2, $3
		)
		on conflict (wallet_id, custodian, linking_id) 
		do update set disconnected_at=null, unlinked_at=null, linked_at=now()
		returning *
	`

	err = tx.Get(cl, stmt, cl.WalletID, cl.Custodian, cl.LinkingID)
	if err != nil {
		sublogger.Error().Err(err).
			Msg("failed to insert wallet_custodian")
		return fmt.Errorf("failed to insert wallet custodian record: %w", err)
	}
	// update wallets with new deposit destination
	stmt = `
		update wallets set
			user_deposit_destination=$1,provider_linking_id=$2,user_deposit_account_provider=$3
		where id=$4
	`
	// perform query
	if r, err := tx.ExecContext(
		ctx,
		stmt,
		depositDest,
		cl.LinkingID,
		cl.Custodian,
		cl.WalletID,
	); err != nil {
		sublogger.Error().Err(err).
			Msg("failed to update wallets with new deposit destination")
		return fmt.Errorf("error updating wallets with new deposit desintation: %w", err)
	} else if r != nil {
		count, _ := r.RowsAffected()
		if count < 1 {
			sublogger.Error().Msg("at least one record should be updated for connecting a verified wallet")
			return errors.New("should have updated at least one wallet for connecting a verified wallet")
		}
	}

	// if the tx was created in this scope we will commit here
	if err := commit(); err != nil {
		return fmt.Errorf("failed to commit ConnectCustodialWallet transaction: %w", err)
	}
	return nil
}

// helper to make logger easier
func logger(ctx context.Context) *zerolog.Logger {
	// get logger
	return logging.Logger(ctx, "wallet")
}

// helper to create a tx
func createTx(ctx context.Context, pg *Postgres) (tx *sqlx.Tx, err error) {
	logger(ctx).Debug().
		Msg("creating transaction")
	tx, err = pg.RawDB().Beginx()
	if err != nil {
		logger(ctx).Error().Err(err).
			Msg("error creating transaction")
		return tx, fmt.Errorf("failed to create transaction: %w", err)
	}
	return tx, nil
}

// acquire tx advisory lock for id automatically released when tx ends
func waitAndLockTx(ctx context.Context, tx *sqlx.Tx, id uuid.UUID) error {
	query := "SELECT pg_advisory_xact_lock(hashtext($1))"
	_, err := tx.ExecContext(ctx, query, id.String())
	if err != nil {
		return fmt.Errorf("failed to acquire tx lock id %s: %w", id.String(), err)
	}
	return nil
}

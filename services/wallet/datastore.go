package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/services/wallet/model"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/clients/reputation"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/datastore"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/logging"
	timeutils "github.com/brave-intl/bat-go/libs/time"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
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
	tenLinkagesReached = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:        "custodian_account_linked_ten_times",
			Help:        "A counter for seeing how many custodian accounts have been linked 10 times",
			ConstLabels: prometheus.Labels{"service": "wallet"},
		})
)

func init() {
	prometheus.MustRegister(tooManyCardsCounter)
	prometheus.MustRegister(metricTxLockGauge)
	prometheus.MustRegister(tenLinkagesReached)
}

// Datastore holds the interface for the wallet datastore
type Datastore interface {
	datastore.Datastore
	LinkWallet(ctx context.Context, id string, providerID string, providerLinkingID uuid.UUID, depositProvider string) error
	GetLinkingLimitInfo(ctx context.Context, providerLinkingID string) (map[string]LinkingInfo, error)
	HasPriorLinking(ctx context.Context, walletID uuid.UUID, providerLinkingID uuid.UUID) (bool, error)
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
	// InsertWalletTx inserts the given wallet as part of provided sql.Tx transaction.
	InsertWalletTx(ctx context.Context, tx *sqlx.Tx, wallet *walletutils.Info) error
	// InsertBitFlyerRequestID - attempt an insert on a request id
	InsertBitFlyerRequestID(ctx context.Context, requestID string) error
	// UpsertWallet UpsertWallets inserts a wallet if it does not already exist
	UpsertWallet(ctx context.Context, wallet *walletutils.Info) error
	// ConnectCustodialWallet - connect the wallet's custodial verified wallet.
	ConnectCustodialWallet(ctx context.Context, cl *CustodianLink, depositDest string) error
	// DisconnectCustodialWallet - disconnect the wallet's custodial id
	DisconnectCustodialWallet(ctx context.Context, walletID uuid.UUID) error
	// GetCustodianLinkByWalletID retrieves the currently linked wallet custodian by walletID.
	GetCustodianLinkByWalletID(ctx context.Context, ID uuid.UUID) (*CustodianLink, error)
	// GetCustodianLinkCount - get the wallet custodian link count across all wallets
	GetCustodianLinkCount(ctx context.Context, linkingID uuid.UUID, custodian string) (int, int, error)
	// InsertVerifiedWalletOutboxTx inserts a verifiedWalletOutbox for processing.
	InsertVerifiedWalletOutboxTx(ctx context.Context, tx *sqlx.Tx, paymentID uuid.UUID, verifiedWallet bool) error
	// SendVerifiedWalletOutbox sends requests to reputation service.
	SendVerifiedWalletOutbox(ctx context.Context, client reputation.Client, retry backoff.RetryFunc) (bool, error)
}

// ReadOnlyDatastore includes all database methods that can be made with a read only db connection
type ReadOnlyDatastore interface {
	datastore.Datastore
	HasPriorLinking(ctx context.Context, walletID uuid.UUID, providerLinkingID uuid.UUID) (bool, error)
	// GetLinkingsByProviderLinkingID gets the wallet linking info by provider linking id
	GetLinkingsByProviderLinkingID(ctx context.Context, providerLinkingID uuid.UUID) ([]LinkingMetadata, error)
	// GetByProviderLinkingID gets a wallet by provider linking id
	GetByProviderLinkingID(ctx context.Context, providerLinkingID uuid.UUID) (*[]walletutils.Info, error)
	// GetWallet by ID
	GetWallet(ctx context.Context, ID uuid.UUID) (*walletutils.Info, error)
	// GetWalletByPublicKey retrieves a wallet by its public key.
	GetWalletByPublicKey(context.Context, string) (*walletutils.Info, error)
	// GetCustodianLinkCount - get the wallet custodian link count across all wallets
	GetCustodianLinkCount(ctx context.Context, linkingID uuid.UUID, custodian string) (int, int, error)
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	datastore.Postgres
}

// NewWritablePostgres creates a new Postgres Datastore
func NewWritablePostgres(databaseURL string, performMigration bool, migrationTrack string, dbStatsPrefix ...string) (Datastore, error) {
	pg, err := datastore.NewPostgres(databaseURL, performMigration, migrationTrack, dbStatsPrefix...)
	if pg != nil {
		return &DatastoreWithPrometheus{
			base: &Postgres{*pg}, instanceName: "wallet_datastore",
		}, err
	}
	return nil, err
}

// NewReadOnlyPostgres creates a new Postgres RO Datastore
func NewReadOnlyPostgres(databaseURL string, performMigration bool, migrationTrack string, dbStatsPrefix ...string) (ReadOnlyDatastore, error) {
	pg, err := datastore.NewPostgres(databaseURL, performMigration, migrationTrack, dbStatsPrefix...)
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
	// ErrNoReputationClient is returned when no reputation client is in the ctx.
	ErrNoReputationClient = errors.New("wallet: no reputation client")
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

// TODO(clD11): address GetWallet in wallet refactor.

// GetWallet retrieves a wallet by its walletID, if no wallet is found then nil is returned.
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

// HasPriorLinking - check if this wallet id has been linked to this provider linking id in the past
func (pg *Postgres) HasPriorLinking(ctx context.Context, walletID uuid.UUID, providerLinkingID uuid.UUID) (bool, error) {
	statement := `
	select exists (
		select 1
		from
			wallet_custodian
		where
			linking_id = $1 and wallet_id = $2
	)
	`
	var existingLinking bool
	err := pg.RawDB().GetContext(ctx, &existingLinking, statement, providerLinkingID, walletID)
	return existingLinking, err
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

// InsertWalletTx inserts the given wallet
func (pg *Postgres) InsertWalletTx(ctx context.Context, tx *sqlx.Tx, wallet *walletutils.Info) error {
	statement := `INSERT INTO wallets (id, provider, provider_id, public_key)	VALUES ($1, $2, $3, $4)`
	_, err := tx.ExecContext(ctx,
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
	case "zebpay":
		if v, err := strconv.Atoi(os.Getenv("ZEBPAY_WALLET_LINKING_LIMIT")); err == nil {
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

var (
	// ErrUnusualActivity - error for wallets with unusual activity
	ErrUnusualActivity = errors.New("unusual activity")
	// ErrGeoResetDifferent - error for wallets with reset geo
	ErrGeoResetDifferent = errors.New("geo reset is different")
)

// LinkWallet links a rewards wallet to the given deposit provider.
func (pg *Postgres) LinkWallet(ctx context.Context, id string, userDepositDestination string, providerLinkingID uuid.UUID, depositProvider string) error {
	walletID, err := uuid.FromString(id)
	if err != nil {
		return fmt.Errorf("invalid wallet id, not uuid: %w", err)
	}

	ctx, tx, rollback, commit, err := getTx(ctx, pg)
	if err != nil {
		return fmt.Errorf("error getting tx: %w", err)
	}
	defer func() {
		metricTxLockGauge.Dec()
		rollback()
	}()

	metricTxLockGauge.Inc()
	if err := waitAndLockTx(ctx, tx, providerLinkingID); err != nil {
		return fmt.Errorf("error acquiring tx lock: %w", err)
	}

	if err := pg.ConnectCustodialWallet(ctx, &CustodianLink{
		WalletID:  &walletID,
		Custodian: depositProvider,
		LinkingID: &providerLinkingID,
	}, userDepositDestination); err != nil {
		return fmt.Errorf("error connect custodian wallet: %w", err)
	}

	// TODO(clD11): the below verified wallets calls were added as a quick fix and should be addressed in the wallet refactor.
	if VerifiedWalletEnable {
		if err := pg.InsertVerifiedWalletOutboxTx(ctx, tx, walletID, true); err != nil {
			return fmt.Errorf("failed to update verified wallet: %w", err)
		}
	}

	if directVerifiedWalletEnable {
		repClient, ok := ctx.Value(appctx.ReputationClientCTXKey).(reputation.Client)
		if !ok {
			return ErrNoReputationClient
		}

		op := func() (interface{}, error) {
			return nil, repClient.UpdateReputationSummary(ctx, walletID.String(), true)
		}

		if _, err := backoff.Retry(ctx, op, retryPolicy, canRetry(nonRetriableErrors)); err != nil {
			return fmt.Errorf("failed to update verified wallet: %w", err)
		}
	}

	if err := commit(); err != nil {
		sentry.CaptureException(fmt.Errorf("error failed to commit link wallet transaction: %w", err))
		return fmt.Errorf("error committing tx: %w", err)
	}

	return nil
}

// TODO(clD11): CustodianLink represent a wallet_custodian. Review during wallet refactor

// CustodianLink representation a wallet_custodian record.
type CustodianLink struct {
	WalletID       *uuid.UUID `json:"wallet_id" db:"wallet_id" valid:"uuidv4"`
	Custodian      string     `json:"custodian" db:"custodian" valid:"in(uphold,brave,gemini,bitflyer)"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at" valid:"-"`
	UpdatedAt      *time.Time `json:"updated_at" db:"updated_at" valid:"-"`
	LinkedAt       time.Time  `json:"linked_at" db:"linked_at" valid:"-"`
	DisconnectedAt *time.Time `json:"disconnected_at" db:"disconnected_at" valid:"-"`
	LinkingID      *uuid.UUID `json:"linking_id" db:"linking_id" valid:"uuid"`
	UnlinkedAt     *time.Time `json:"unlinked_at" db:"unlinked_at" valid:"-"`
}

// TODO(clD11): Wallet Refactor. These should not be nullable, fix pointers and raname fields for consistency.

func NewSolanaCustodialLink(walletID uuid.UUID, depositDestination string) *CustodianLink {
	const depositProviderSolana = "solana"
	return &CustodianLink{
		WalletID:  &walletID,
		LinkingID: ptrFromUUID(uuid.NewV5(ClaimNamespace, depositDestination)),
		Custodian: depositProviderSolana,
	}
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

func (cl *CustodianLink) isLinked() bool {
	return cl != nil && cl.UnlinkedAt == nil && cl.DisconnectedAt == nil && !cl.LinkedAt.IsZero()
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

func rollbackFn(ctx context.Context, datastore Datastore, tx *sqlx.Tx) func() {
	return func() {
		logger(ctx).Debug().Msg("rolling back transaction")
		datastore.RollbackTx(tx)
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
func getTx(ctx context.Context, datastore Datastore) (context.Context, *sqlx.Tx, func(), func() error, error) {
	tx, noContextTx := ctx.Value(appctx.DatabaseTransactionCTXKey).(*sqlx.Tx)
	if !noContextTx {
		tx, err := createTx(ctx, datastore)
		if err != nil || tx == nil {
			return ctx, nil, func() {}, func() error { return nil }, fmt.Errorf("failed to create tx: %w", err)
		}
		ctx = context.WithValue(ctx, appctx.DatabaseTransactionCTXKey, tx)
		return ctx, tx, rollbackFn(ctx, datastore, tx), commitFn(ctx, tx), nil
	}
	return ctx, tx, func() {}, func() error { return nil }, nil
}

// GetCustodianLinkByWalletID retrieves the currently linked wallet custodian by walletID.
func (pg *Postgres) GetCustodianLinkByWalletID(ctx context.Context, ID uuid.UUID) (*CustodianLink, error) {
	const q = `
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
	result := &CustodianLink{}
	if err := pg.GetContext(ctx, result, q, ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNoWalletCustodian
		}
		return nil, err
	}

	return result, nil
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
			user_deposit_destination='',
			user_deposit_account_provider=null
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
			disconnected_at=now(),
			updated_at=now()
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

	// TODO(clD11): WR. The relinking check below only considers the currently linked wallet and not
	//  the custodian. We can combine/refactor these checks for both linkings, custodians and limits.

	if err := validateCustodianLinking(ctx, pg, *cl.WalletID, cl.Custodian); err != nil {
		if errors.Is(err, errCustodianLinkMismatch) {
			return errCustodianLinkMismatch
		}
		return handlers.WrapError(err, "failed to check linking mismatch", http.StatusInternalServerError)
	}

	// Relinking.
	if !uuid.Equal(existingLinkingID, *new(uuid.UUID)) { // if not a new wallet
		// check if the currently linked wallet matches the proposed linking.
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

		// this will be the 10th linking
		if used == 9 {
			defer tenLinkagesReached.Inc()
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

	// evict any prior linkings that were not disconnected (across all custodians)
	stmt = `
		update wallet_custodian set disconnected_at=now(), updated_at=now() where wallet_id=$1 and disconnected_at is null
	`
	// perform query
	if _, err := tx.ExecContext(ctx, stmt, cl.WalletID); err != nil {
		sublogger.Error().Err(err).
			Msg("failed to update wallet_custodian evicting prior linked")
		return fmt.Errorf("error updating wallet_custodian evicting prior linked: %w", err)
	}

	stmt = `
		insert into wallet_custodian (
			wallet_id, custodian, linking_id
		) values (
			$1, $2, $3
		)
		on conflict (wallet_id, custodian, linking_id)
		do update set updated_at=now(), disconnected_at=null, unlinked_at=null, linked_at=now()
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

// InsertVerifiedWalletOutboxTx inserts a verifiedWalletOutbox for processing.
func (pg *Postgres) InsertVerifiedWalletOutboxTx(ctx context.Context, tx *sqlx.Tx, walletID uuid.UUID, verifiedWallet bool) error {
	_, err := tx.ExecContext(ctx, `insert into verified_wallet_outbox(payment_id, verified_wallet)
											values ($1, $2)`, walletID, verifiedWallet)
	if err != nil {
		return fmt.Errorf("error inserting values into vefified wallet outbox: %w", err)
	}
	return nil
}

// SendVerifiedWalletOutbox sends requests to reputation service.
func (pg *Postgres) SendVerifiedWalletOutbox(ctx context.Context, client reputation.Client, retry backoff.RetryFunc) (bool, error) {
	vw := struct {
		ID             uuid.UUID `db:"id"`
		PaymentID      uuid.UUID `db:"payment_id"`
		VerifiedWallet bool      `db:"verified_wallet"`
	}{}

	_, tx, rollback, commit, err := datastore.GetTx(ctx, pg)
	if err != nil {
		return false, fmt.Errorf("error getting tx for verified wallet: %w", err)
	}
	defer rollback()

	err = tx.Get(&vw, `select id, payment_id, verified_wallet from verified_wallet_outbox
                                   order by created_at asc for update skip locked limit 1`)
	if err != nil {
		return false, fmt.Errorf("error get verified wallet: %w", err)
	}

	upsertReputationSummaryOp := func() (interface{}, error) {
		return nil, client.UpdateReputationSummary(ctx, vw.PaymentID.String(), vw.VerifiedWallet)
	}

	_, err = retry(ctx, upsertReputationSummaryOp, retryPolicy, canRetry(nonRetriableErrors))
	if err != nil {
		return true, fmt.Errorf("error calling reputation for verified wallet: %w", err)
	}

	_, err = tx.ExecContext(ctx, "delete from verified_wallet_outbox where id = $1", vw.ID)
	if err != nil {
		return true, fmt.Errorf("error deleting verified wallet txn: %w", err)
	}

	err = commit()
	if err != nil {
		return true, fmt.Errorf("error commit verified wallet txn: %w", err)
	}

	return true, nil
}

func validateCustodianLinking(ctx context.Context, storage Datastore, walletID uuid.UUID, depositProvider string) error {
	c, err := storage.GetCustodianLinkByWalletID(ctx, walletID)
	if err != nil && !errors.Is(err, model.ErrNoWalletCustodian) {
		return err
	}

	// if there are no instances of wallet custodian then it is
	// considered a new linking and therefore valid.
	if c == nil {
		return nil
	}

	if !strings.EqualFold(c.Custodian, depositProvider) {
		return errCustodianLinkMismatch
	}

	return nil
}

func logger(ctx context.Context) *zerolog.Logger {
	return logging.Logger(ctx, "wallet")
}

// helper to create a tx
func createTx(_ context.Context, datastore Datastore) (tx *sqlx.Tx, err error) {
	tx, err = datastore.RawDB().Beginx()
	if err != nil {
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

func ptrFromUUID(u uuid.UUID) *uuid.UUID {
	return &u
}

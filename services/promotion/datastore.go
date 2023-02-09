package promotion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/brave-intl/bat-go/libs/clients/gemini"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/ptr"

	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/clients/cbr"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/datastore"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/jsonutils"
	"github.com/brave-intl/bat-go/libs/logging"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/getsentry/sentry-go"
	"github.com/lib/pq"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var desktopPlatforms = [...]string{"linux", "osx", "windows"}

var (
	// metric for claim drains status
	// custodians are gemini, bitflyer, uphold and unknown
	// status are complete and failed
	countClaimDrainStatus = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "count_claim_drain_status",
			Help: "provides a count of the complete and failed claim drains partitioned by custodian and status",
		},
		[]string{"custodian", "status"},
	)
	// counter for flagged unusual
	countDrainFlaggedUnusual = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:        "count_drain_flagged_unusual",
			Help:        "provides a count of unusual drain flagged results",
			ConstLabels: prometheus.Labels{"service": "promotions"},
		})
)

func init() {
	prometheus.MustRegister(countClaimDrainStatus)
	prometheus.MustRegister(withdrawalLimitHit)
	prometheus.MustRegister(countDrainFlaggedUnusual)
}

// ClobberedCreds holds data of claims that have been clobbered and when they were first reported
type ClobberedCreds struct {
	ID        uuid.UUID `db:"id"`
	CreatedAt time.Time `db:"created_at"`
	Version   int       `db:"version"`
}

// BATLossEvent holds info about wallet events
type BATLossEvent struct {
	ID       uuid.UUID       `db:"id" json:"id"`
	WalletID uuid.UUID       `db:"wallet_id" json:"walletId"`
	ReportID int             `db:"report_id" json:"reportId"`
	Amount   decimal.Decimal `db:"amount" json:"amount"`
	Platform string          `db:"platform" json:"platform"`
}

// DrainClaim holds drain claim data
type DrainClaim struct {
	BatchID     *uuid.UUID
	Claim       *Claim
	Credentials []cbr.CredentialRedemption
	Wallet      *walletutils.Info
	Total       decimal.Decimal
	CodedErr    errorutils.DrainCodified
}

// Datastore abstracts over the underlying datastore
type Datastore interface {
	datastore.Datastore
	// ActivatePromotion marks a particular promotion as active
	ActivatePromotion(promotion *Promotion) error
	// DeactivatePromotion marks a particular promotion as inactive
	DeactivatePromotion(promotion *Promotion) error
	// ClaimForWallet is used to either create a new claim or convert a preregistered claim for a particular promotion
	ClaimForWallet(promotion *Promotion, issuer *Issuer, wallet *walletutils.Info, blindedCreds jsonutils.JSONStringArray) (*Claim, error)
	// CreateClaim is used to "pre-register" an unredeemed claim for a particular wallet
	CreateClaim(promotionID uuid.UUID, walletID string, value decimal.Decimal, bonus decimal.Decimal, legacy bool) (*Claim, error)
	// GetPreClaim is used to fetch a "pre-registered" claim for a particular wallet
	GetPreClaim(promotionID uuid.UUID, walletID string) (*Claim, error)
	// CreatePromotion given the promotion type, initial number of grants and the desired value of those grants
	CreatePromotion(promotionType string, numGrants int, value decimal.Decimal, platform string) (*Promotion, error)
	// GetAvailablePromotionsForWallet returns the list of available promotions for the wallet
	GetAvailablePromotionsForWallet(wallet *walletutils.Info, platform string) ([]Promotion, error)
	// GetAvailablePromotions returns the list of available promotions for all wallets
	GetAvailablePromotions(platform string) ([]Promotion, error)
	// GetWithdrawalsAssociated returns the promotion and total amount of claims drained for associated wallets
	GetWithdrawalsAssociated(walletID, claimID *uuid.UUID) (*uuid.UUID, decimal.Decimal, error)
	// GetPromotionsMissingIssuer returns the list of promotions missing an issuer
	GetPromotionsMissingIssuer(limit int) ([]uuid.UUID, error)
	// GetClaimCreds returns the claim credentials for a ClaimID
	GetClaimCreds(claimID uuid.UUID) (*ClaimCreds, error)
	// SaveClaimCreds updates the stored claim credentials
	SaveClaimCreds(claimCreds *ClaimCreds) error
	// GetPromotion by ID
	GetPromotion(promotionID uuid.UUID) (*Promotion, error)
	// InsertIssuer inserts the given issuer
	InsertIssuer(issuer *Issuer) (*Issuer, error)
	// GetIssuer by PromotionID and cohort
	GetIssuer(promotionID uuid.UUID, cohort string) (*Issuer, error)
	// GetIssuerByPublicKey
	GetIssuerByPublicKey(publicKey string) (*Issuer, error)
	// GetClaimSummary gets the number of grants for a specific type
	GetClaimSummary(walletID uuid.UUID, grantType string) (*ClaimSummary, error)
	// GetClaimByWalletAndPromotion gets whether a wallet has a claimed grants
	// with the given promotion and returns the grant if so
	GetClaimByWalletAndPromotion(wallet *walletutils.Info, promotionID *Promotion) (*Claim, error)
	// RunNextClaimJob to sign claim credentials if there is a claim waiting
	RunNextClaimJob(ctx context.Context, worker ClaimWorker) (bool, error)
	// InsertSuggestion inserts a transaction awaiting validation
	InsertSuggestion(credentials []cbr.CredentialRedemption, suggestionText string, suggestion []byte) error
	// RunNextSuggestionJob to process a suggestion if there is one waiting
	RunNextSuggestionJob(ctx context.Context, worker SuggestionWorker) (bool, error)
	// InsertClobberedClaims inserts clobbered claim ids into the clobbered_claims table
	InsertClobberedClaims(ctx context.Context, ids []uuid.UUID, version int) error
	// InsertBATLossEvent inserts claims of lost bat
	InsertBATLossEvent(ctx context.Context, paymentID uuid.UUID, reportID int, amount decimal.Decimal, platform string) (bool, error)
	// InsertBAPReportEvent inserts a BAP report
	InsertBAPReportEvent(ctx context.Context, paymentID uuid.UUID, amount decimal.Decimal) (*uuid.UUID, error)
	// DrainClaim by marking the claim as drained and inserting a new drain entry
	DrainClaim(drainID *uuid.UUID, claim *Claim, credentials []cbr.CredentialRedemption, wallet *walletutils.Info, total decimal.Decimal, codedErr errorutils.DrainCodified) error
	// InsertBatchDrainClaim insert drain claims
	DrainClaims(drainClaims []DrainClaim) error
	// RunNextDrainJob to process deposits if there is one waiting
	RunNextDrainJob(ctx context.Context, worker DrainWorker) (bool, error)
	// RunNextDrainRetryJob toggles failed drain jobs to be reprocessed if eligible
	RunNextDrainRetryJob(ctx context.Context, worker DrainRetryWorker) error
	// EnqueueMintDrainJob - enqueue a mint drain job in "pending" status
	EnqueueMintDrainJob(ctx context.Context, walletID uuid.UUID, promotionIDs ...uuid.UUID) error
	// SetMintDrainPromotionTotal - set the per promotion total for the mint drain
	SetMintDrainPromotionTotal(ctx context.Context, walletID, promotionID uuid.UUID, total decimal.Decimal) error
	// RunNextMintDrainJob to create new grants from the mint queue
	RunNextMintDrainJob(ctx context.Context, worker MintWorker) (bool, error)
	// RunNextGeminiCheckStatus periodically check the status of gemini claim drain transactions
	RunNextGeminiCheckStatus(ctx context.Context, worker GeminiTxnStatusWorker) (bool, error)

	// Remove once this is completed https://github.com/brave-intl/bat-go/issues/263

	// GetOrder by ID
	GetOrder(orderID uuid.UUID) (*Order, error)
	// UpdateOrder updates an order when it has been paid
	UpdateOrder(orderID uuid.UUID, status string) error
	// CreateTransaction creates a transaction
	CreateTransaction(orderID uuid.UUID, externalTransactionID string, status string, currency string, kind string, amount decimal.Decimal) (*Transaction, error)
	// GetSumForTransactions gets a decimal sum of for transactions for an order
	GetSumForTransactions(orderID uuid.UUID) (decimal.Decimal, error)
	// GetDrainPoll gets the information about a drain poll job
	GetDrainPoll(drainID *uuid.UUID) (*DrainPoll, error)
	// GetDrainsByBatchID gets the information about a drain poll job
	GetDrainsByBatchID(ctx context.Context, batchID *uuid.UUID) ([]DrainTransfer, error)
	// GetCustodianDrainInfo gets the information about a drain poll job
	GetCustodianDrainInfo(paymentID *uuid.UUID) ([]CustodianDrain, error)
	// RunNextBatchPaymentsJob to sign claim credentials if there is a claim waiting
	RunNextBatchPaymentsJob(ctx context.Context, worker BatchTransferWorker) (bool, error)
	// UpdateDrainJobErred - manually update drain job for retry
	UpdateDrainJobAsRetriable(ctx context.Context, walletID uuid.UUID) error
}

// ReadOnlyDatastore includes all database methods that can be made with a read only db connection
type ReadOnlyDatastore interface {
	datastore.Datastore
	// GetPreClaim is used to fetch a "pre-registered" claim for a particular wallet
	GetPreClaim(promotionID uuid.UUID, walletID string) (*Claim, error)
	// GetAvailablePromotionsForWallet returns the list of available promotions for the wallet
	GetAvailablePromotionsForWallet(wallet *walletutils.Info, platform string) ([]Promotion, error)
	// GetWithdrawalsAssociated returns the promotion and total amount of claims drained for associated wallets
	GetWithdrawalsAssociated(walletID, claimID *uuid.UUID) (*uuid.UUID, decimal.Decimal, error)
	// GetAvailablePromotions returns the list of available promotions for all wallets
	GetAvailablePromotions(platform string) ([]Promotion, error)
	// GetPromotionsMissingIssuer returns the list of promotions missing an issuer
	GetPromotionsMissingIssuer(limit int) ([]uuid.UUID, error)
	// GetClaimCreds returns the claim credentials for a ClaimID
	GetClaimCreds(claimID uuid.UUID) (*ClaimCreds, error)
	// GetPromotion by ID
	GetPromotion(promotionID uuid.UUID) (*Promotion, error)
	// GetIssuer by PromotionID and cohort
	GetIssuer(promotionID uuid.UUID, cohort string) (*Issuer, error)
	// GetIssuerByPublicKey
	GetIssuerByPublicKey(publicKey string) (*Issuer, error)
	// GetClaimSummary gets the number of grants for a specific type
	GetClaimSummary(walletID uuid.UUID, grantType string) (*ClaimSummary, error)
	// GetClaimByWalletAndPromotion gets whether a wallet has a claimed grants
	// with the given promotion and returns the grant if so
	GetClaimByWalletAndPromotion(wallet *walletutils.Info, promotionID *Promotion) (*Claim, error)
	// GetDrainPoll gets the information about a drain poll job
	GetDrainPoll(drainID *uuid.UUID) (*DrainPoll, error)
	// GetCustodianDrainInfo gets the information about a drain poll job
	GetCustodianDrainInfo(paymentID *uuid.UUID) ([]CustodianDrain, error)
	// GetDrainsByBatchID gets the information about a drain poll job
	GetDrainsByBatchID(ctx context.Context, batchID *uuid.UUID) ([]DrainTransfer, error)
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	datastore.Postgres
}

// NewDB creates a new Postgres Datastore
func NewDB(databaseURL string, performMigration bool, migrationTrack string, dbStatsPrefix ...string) (Datastore, error) {
	pg, err := datastore.NewPostgres(databaseURL, performMigration, migrationTrack, dbStatsPrefix...)
	if pg != nil {
		return &DatastoreWithPrometheus{
			base: &Postgres{*pg}, instanceName: "promotion_datastore",
		}, err
	}
	return nil, err
}

// NewRODB creates a new Postgres RO Datastore
func NewRODB(databaseURL string, performMigration bool, migrationTrack string, dbStatsPrefix ...string) (ReadOnlyDatastore, error) {
	pg, err := datastore.NewPostgres(databaseURL, performMigration, migrationTrack, dbStatsPrefix...)
	if pg != nil {
		return &ReadOnlyDatastoreWithPrometheus{
			base: &Postgres{*pg}, instanceName: "promotion_ro_datastore",
		}, err
	}
	return nil, err
}

// NewPostgres creates new postgres connections
func NewPostgres() (Datastore, ReadOnlyDatastore, error) {
	var roPg ReadOnlyDatastore
	pg, err := NewDB("", true, "promotion", "promotion_db")
	if err != nil {
		sentry.CaptureException(err)
		log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
	}
	roDB := os.Getenv("RO_DATABASE_URL")
	if len(roDB) > 0 {
		roPg, err = NewRODB(roDB, false, "promotion", "promotion_read_only_db")
		if err != nil {
			sentry.CaptureException(err)
			log.Error().Err(err).Msg("Could not start reader postgres connection")
		}
	}
	if roPg == nil {
		roPg = pg
	}
	return pg, roPg, err
}

// CreatePromotion given the promotion type, initial number of grants and the desired value of those grants
func (pg *Postgres) CreatePromotion(promotionType string, numGrants int, value decimal.Decimal, platform string) (*Promotion, error) {
	statement := `
	insert into promotions (promotion_type, remaining_grants, approximate_value, suggestions_per_grant, platform)
	values ($1, $2, $3, $4, $5)
	returning *`
	promotions := []Promotion{}
	suggestionsPerGrant := value.Div(defaultVoteValue)
	err := pg.RawDB().Select(&promotions, statement, promotionType, numGrants, value, suggestionsPerGrant, platform)
	if err != nil {
		return nil, err
	}

	return &promotions[0], nil
}

// GetPromotion by ID
func (pg *Postgres) GetPromotion(promotionID uuid.UUID) (*Promotion, error) {
	statement := `select *,
		case when claimable_until_override is null then created_at + interval '3 months'
			else claimable_until_override
		end as claimable_until
		from promotions where id = $1`
	promotions := []Promotion{}
	err := pg.RawDB().Select(&promotions, statement, promotionID)
	if err != nil {
		return nil, err
	}

	if len(promotions) > 0 {
		return &promotions[0], nil
	}

	return nil, nil
}

// InsertClobberedClaims inserts clobbered claims to the db
func (pg *Postgres) InsertClobberedClaims(ctx context.Context, ids []uuid.UUID, version int) error {
	tx, err := pg.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer pg.RollbackTx(tx)

	if version < 2 {
		// if no version, assume 1
		version = 1
	}

	for _, id := range ids {
		_, err = tx.Exec(`INSERT INTO clobbered_claims (id, version) values ($1, $2) ON CONFLICT DO NOTHING;`, id, version)
		if err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}

// DrainTransfer info about the drains
type DrainTransfer struct {
	ID        *uuid.UUID      `db:"transaction_id" json:"transaction_id"`
	Total     decimal.Decimal `db:"total" json:"total"`
	DepositID *string         `db:"deposit_destination" json:"deposit_destination"`
}

// BAPReport holds info about wallet events
type BAPReport struct {
	ID        uuid.UUID       `db:"id" json:"id"`
	WalletID  uuid.UUID       `db:"wallet_id" json:"walletId"`
	Amount    decimal.Decimal `db:"amount" json:"amount"`
	CreatedAt time.Time       `db:"created_at" json:"createdAt"`
}

// InsertBAPReportEvent inserts a BAP report
func (pg *Postgres) InsertBAPReportEvent(ctx context.Context, paymentID uuid.UUID, amount decimal.Decimal) (*uuid.UUID, error) {

	// bap report id
	id := uuid.NewV4()

	tx, err := pg.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer pg.RollbackTx(tx)

	insertBapReportEventStatement := `
INSERT INTO bap_report (id, wallet_id, amount)
VALUES ($1, $2, $3)`

	_, err = tx.Exec(
		insertBapReportEventStatement,
		id,
		paymentID,
		amount,
	)
	if err != nil {
		// if this is a duplicate constraint error, conflict propogation
		var pgErr *pq.Error
		if errors.As(err, &pgErr) {
			if pgErr.Code == pq.ErrorCode("23505") {
				// duplicate
				return nil, errorutils.ErrConflictBAPReportEvent
			}
		}
		return nil, err
	}
	err = tx.Commit()
	return &id, err
}

// InsertBATLossEvent inserts claims of lost bat to db
func (pg *Postgres) InsertBATLossEvent(
	ctx context.Context,
	paymentID uuid.UUID,
	reportID int,
	amount decimal.Decimal,
	platform string,
) (bool, error) {
	tx, err := pg.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer pg.RollbackTx(tx)
	BATLossEvents := []BATLossEvent{}

	selectStatement := `
SELECT *
FROM bat_loss_events
WHERE wallet_id = $1
	AND report_id = $2`

	insertBATLossEventStatement := `
INSERT INTO bat_loss_events (wallet_id, report_id, amount, platform)
VALUES ($1, $2, $3, $4)`

	err = tx.Select(
		&BATLossEvents,
		selectStatement,
		paymentID.String(),
		reportID,
	)
	if err != nil {
		return false, err
	}
	if len(BATLossEvents) == 0 {
		_, err = tx.Exec(
			insertBATLossEventStatement,
			paymentID.String(),
			reportID,
			amount,
			platform,
		)
		if err != nil {
			return false, err
		}
	} else {
		if !amount.Equal(BATLossEvents[0].Amount) {
			return false, errorutils.ErrConflictBATLossEvent
		}
		return false, nil
	}
	err = tx.Commit()
	return true, err
}

// ActivatePromotion marks a particular promotion as active
func (pg *Postgres) ActivatePromotion(promotion *Promotion) error {
	return pg.setPromotionActive(promotion, true)
}

// DeactivatePromotion marks a particular promotion as not active
func (pg *Postgres) DeactivatePromotion(promotion *Promotion) error {
	return pg.setPromotionActive(promotion, false)
}

// setPromotionActive marks a particular promotion's active value
func (pg *Postgres) setPromotionActive(promotion *Promotion, active bool) error {
	_, err := pg.RawDB().Exec("update promotions set active = $2 where id = $1", promotion.ID, active)
	if err != nil {
		return err
	}

	return nil
}

// InsertIssuer inserts the given issuer
func (pg *Postgres) InsertIssuer(issuer *Issuer) (*Issuer, error) {
	statement := `
	insert into issuers (promotion_id, cohort, public_key)
	values ($1, $2, $3)
	returning *`
	issuers := []Issuer{}
	err := pg.RawDB().Select(&issuers, statement, issuer.PromotionID, issuer.Cohort, issuer.PublicKey)
	if err != nil {
		return nil, err
	}

	if len(issuers) != 1 {
		return nil, errors.New("unexpected number of issuers returned")
	}

	return &issuers[0], nil
}

// GetIssuer by PromotionID and cohort
func (pg *Postgres) GetIssuer(promotionID uuid.UUID, cohort string) (*Issuer, error) {
	statement := "select * from issuers where promotion_id = $1 and cohort = $2"
	issuers := []Issuer{}
	err := pg.RawDB().Select(&issuers, statement, promotionID.String(), cohort)
	if err != nil {
		return nil, err
	}

	if len(issuers) > 0 {
		return &issuers[0], nil
	}

	return nil, nil
}

// GetIssuerByPublicKey or return an error
func (pg *Postgres) GetIssuerByPublicKey(publicKey string) (*Issuer, error) {
	statement := "select * from issuers where public_key = $1"
	issuers := []Issuer{}
	err := pg.RawDB().Select(&issuers, statement, publicKey)
	if err != nil {
		return nil, err
	}

	if len(issuers) > 0 {
		return &issuers[0], nil
	}

	return nil, nil
}

// CreateClaim is used to "pre-register" an unredeemed claim for a particular wallet
func (pg *Postgres) CreateClaim(promotionID uuid.UUID, walletID string, value decimal.Decimal, bonus decimal.Decimal, legacy bool) (*Claim, error) {
	statement := `
	insert into claims (promotion_id, wallet_id, approximate_value, bonus, legacy_claimed)
	values ($1, $2, $3, $4, $5)
	returning *`
	claims := []Claim{}
	err := pg.RawDB().Select(&claims, statement, promotionID, walletID, value, bonus, legacy)
	if err != nil {
		return nil, err
	}

	return &claims[0], nil
}

// GetPreClaim is used to fetch a "pre-registered" claim for a particular wallet
func (pg *Postgres) GetPreClaim(promotionID uuid.UUID, walletID string) (*Claim, error) {
	claims := []Claim{}
	err := pg.RawDB().Select(&claims, "select * from claims where promotion_id = $1 and wallet_id = $2", promotionID.String(), walletID)
	if err != nil {
		return nil, err
	}

	if len(claims) > 0 {
		return &claims[0], nil
	}

	return nil, nil
}

// ClaimForWallet is used to either create a new claim or convert a preregistered claim for a particular promotion
func (pg *Postgres) ClaimForWallet(promotion *Promotion, issuer *Issuer, wallet *walletutils.Info, blindedCreds jsonutils.JSONStringArray) (*Claim, error) {
	blindedCredsJSON, err := json.Marshal(blindedCreds)
	if err != nil {
		return nil, err
	}

	if promotion.ExpiresAt.Before(time.Now().UTC()) {
		return nil, errors.New("unable to claim expired promotion")
	}

	tx, err := pg.RawDB().Beginx()
	if err != nil {
		return nil, err
	}
	defer pg.RollbackTx(tx)

	claims := []Claim{}

	// Get legacy claims
	err = tx.Select(&claims, `select * from claims where legacy_claimed and promotion_id = $1 and wallet_id = $2`, promotion.ID, wallet.ID)
	if err != nil {
		return nil, err
	}

	legacyClaimExists := false
	if len(claims) > 1 {
		panic("impossible number of claims")
	} else if len(claims) == 1 {
		legacyClaimExists = true
	}

	if !legacyClaimExists {
		// This will error if remaining_grants is insufficient due to constraint or the promotion is inactive
		res, err := tx.Exec(`
			update promotions
			set remaining_grants = remaining_grants - 1
			where
				id = $1 and
				active and
				promotions.created_at > NOW() - INTERVAL '3 months'`,
			promotion.ID)

		if err != nil {
			return nil, err
		}
		promotionCount, err := res.RowsAffected()
		if err != nil {
			return nil, err
		} else if promotionCount != 1 {
			return nil, errors.New("no matching active promotion")
		}
	}

	claims = []Claim{}

	if promotion.Type == "ads" || legacyClaimExists {
		statement := `
		update claims
		set redeemed = true, redeemed_at = now()
		where promotion_id = $1 and wallet_id = $2 and not redeemed
		returning *`
		err = tx.Select(&claims, statement, promotion.ID, wallet.ID)
	} else {
		statement := `
		insert into claims (promotion_id, wallet_id, approximate_value, redeemed, redeemed_at)
		values ($1, $2, $3, true, now())
		returning *`
		err = tx.Select(&claims, statement, promotion.ID, wallet.ID, promotion.ApproximateValue)
	}

	if err != nil {
		return nil, err
	} else if len(claims) != 1 {
		return nil, fmt.Errorf("incorrect number of claims updated / inserted: %d", len(claims))
	}
	claim := claims[0]

	// This will error if user has already claimed due to uniqueness constraint
	_, err = tx.Exec(`insert into claim_creds (issuer_id, claim_id, blinded_creds, created_at) values ($1, $2, $3, now()) on conflict do nothing`, issuer.ID, claim.ID, blindedCredsJSON)
	if err != nil {
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return &claim, nil
}

// GetWithdrawalsAssociated returns the promotion and total amount of claims drained for associated wallets
func (pg *Postgres) GetWithdrawalsAssociated(walletID, claimID *uuid.UUID) (*uuid.UUID, decimal.Decimal, error) {

	type associatedWithdrawals struct {
		PromotionID      *uuid.UUID      `db:"promotion_id"`
		WithdrawalAmount decimal.Decimal `db:"withdrawal_amount"`
	}

	var (
		stmt = `
		select
			promotion_id,sum(approximate_value) as withdrawal_amount
		from
			claims
		where
			drained=true and
			wallet_id in (select wallet_id from wallet_custodian where linking_id in (select linking_id from wallet_custodian where wallet_id = $1 )) and
			promotion_id= (select promotion_id from claims where id= $2 limit 1)
		group by
			promotion_id;
		`
		result = new(associatedWithdrawals)
	)

	var err = pg.RawDB().Get(result, stmt, walletID, claimID)
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("failed to get withdrawal amount: %w", err)
	}

	// TODO: implement, get the promotion id and the total amount withdrawn for associated wallets
	return result.PromotionID, result.WithdrawalAmount, nil
}

// GetAvailablePromotionsForWallet returns the list of available promotions for the wallet
func (pg *Postgres) GetAvailablePromotionsForWallet(wallet *walletutils.Info, platform string) ([]Promotion, error) {
	for _, desktopPlatform := range desktopPlatforms {
		if platform == desktopPlatform {
			platform = "desktop"
		}
	}
	statement := `
		select
			promos.id,
			promos.promotion_type,
			promos.created_at,
			case when claimable_until_override is null then promos.created_at + interval '3 months'
				else claimable_until_override
			end as claimable_until,
			promos.expires_at,
			promos.version,
			coalesce(wallet_claims.approximate_value, promos.approximate_value) as approximate_value,
			greatest(1, (coalesce(wallet_claims.approximate_value, promos.approximate_value) /
					promos.approximate_value *
					promos.suggestions_per_grant
				)::int) as suggestions_per_grant,
			promos.remaining_grants,
			promos.platform,
			promos.active,
			promos.public_keys,
			coalesce(wallet_claims.legacy_claimed, false) as legacy_claimed,
			true as available
		from
			(
				select * from 
					(
						select
						promotion_id,
						array_to_json(array_remove(array_agg(public_key), null)) as public_keys
						from issuers
						group by promotion_id
					) issuer_keys join promotions on promotions.id = issuer_keys.promotion_id
						where ( promotions.platform = '' or promotions.platform = $2)
			) promos left join (
				select * from claims where claims.wallet_id = $1
			) wallet_claims on promos.id = wallet_claims.promotion_id
		where
			(wallet_claims.redeemed is distinct from true or promos.id='66d1d59f-2c12-44d6-810e-6a3b87fcb9e8' ) and (
				wallet_claims.legacy_claimed is true or (
					promos.created_at > NOW() - INTERVAL '3 months' and promos.active and (
						( promos.promotion_type = 'ugp' and promos.remaining_grants > 0 ) or
						( promos.promotion_type = 'ads' and wallet_claims.id is not null )
					)
				)
			)
		order by promos.created_at;`
	// TODO: remove the promos.id hardcode in 3 months

	promotions := []Promotion{}

	err := pg.RawDB().Select(&promotions, statement, wallet.ID, platform)
	if err != nil {
		return promotions, err
	}

	return promotions, nil
}

// GetAvailablePromotions returns the list of available promotions for all wallets
func (pg *Postgres) GetAvailablePromotions(platform string) ([]Promotion, error) {
	for _, desktopPlatform := range desktopPlatforms {
		if platform == desktopPlatform {
			platform = "desktop"
		}
	}
	statement := `
		select
			promotions.*,
			case when promotions.claimable_until_override is null then promotions.created_at + interval '3 months'
				else claimable_until_override
			end as claimable_until,
			false as legacy_claimed,
			true as available,
			array_to_json(array_remove(array_agg(issuers.public_key), null)) as public_keys
		from
		promotions left join issuers on promotions.id = issuers.promotion_id
		where promotions.promotion_type = 'ugp' and
			( promotions.platform = '' or promotions.platform = $1) and
			promotions.active and promotions.remaining_grants > 0
		group by promotions.id
		order by promotions.created_at;`

	promotions := []Promotion{}

	err := pg.RawDB().Select(&promotions, statement, platform)
	if err != nil {
		return promotions, err
	}

	return promotions, nil
}

// GetPromotionsMissingIssuer returns the list of promotions missing an issuer
func (pg *Postgres) GetPromotionsMissingIssuer(limit int) ([]uuid.UUID, error) {
	var (
		resp      = []uuid.UUID{}
		statement = `
		select
			promotions.id
		from
			promotions left join issuers
			on promotions.id = issuers.promotion_id
		where
			issuers.public_key is null
		limit $1`
	)

	err := pg.RawDB().Select(&resp, statement, limit)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// GetClaimCreds returns the claim credentials for a ClaimID
func (pg *Postgres) GetClaimCreds(claimID uuid.UUID) (*ClaimCreds, error) {
	claimCreds := []ClaimCreds{}
	err := pg.RawDB().Select(&claimCreds, "select * from claim_creds where claim_id = $1", claimID)
	if err != nil {
		return nil, err
	}

	if len(claimCreds) > 0 {
		return &claimCreds[0], nil
	}

	return nil, nil
}

// SaveClaimCreds updates the stored claim credentials
func (pg *Postgres) SaveClaimCreds(creds *ClaimCreds) error {
	_, err := pg.RawDB().Exec(`update claim_creds set signed_creds = $1, batch_proof = $2, public_key = $3, updated_at= now() where claim_id = $4`, creds.SignedCreds, creds.BatchProof, creds.PublicKey, creds.ID)
	return err
}

// MarkBatchTransferSubmitted mark this batch of transfers submitted
func (pg *Postgres) MarkBatchTransferSubmitted(ctx context.Context, batchID *uuid.UUID) error {
	tx, err := pg.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer pg.RollbackTx(tx)

	stmt := "update claim_drain set status = 'complete', completed=true, completed_at = now() where batch_id = $1"
	if _, err := tx.Exec(stmt, batchID); err == nil {
		return tx.Commit()
	}
	return fmt.Errorf("failed to mark batch transfer submitted: %w", err)
}

// GetCustodianDrainInfo Get the status of the custodian drain info
func (pg *Postgres) GetCustodianDrainInfo(paymentID *uuid.UUID) ([]CustodianDrain, error) {
	resp := []CustodianDrain{}
	// get the linked wallet info
	stmt := `
select
	user_deposit_account_provider, user_deposit_destination
from
	wallets
where
	id = $1
`
	var custodian Custodian
	if err := pg.RawDB().Get(&custodian, stmt, paymentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	// get all of the drain jobs for this payment id
	stmt = `
select
	batch_id,
	split_part(credentials->0->>'issuer',':',1) as promotion_id,
	completed_at,
	json_array_length(credentials)*0.25 as value,
	case when erred then 'errored' else 'succeeded' end as state,
	errcode,
	transaction_id
from
	claim_drain
where
	wallet_id = $1
`
	type batchedPromotionsDrained struct {
		DrainInfo
		BatchID uuid.UUID `db:"batch_id"`
	}

	var promosDrained = []batchedPromotionsDrained{}
	if err := pg.RawDB().Select(&promosDrained, stmt, paymentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	batches := map[uuid.UUID][]DrainInfo{}
	batchValue := map[uuid.UUID]decimal.Decimal{}

	// chunk all these into related batches
	for i := 0; i < len(promosDrained); i++ {
		if _, ok := batches[promosDrained[i].BatchID]; !ok {
			batches[promosDrained[i].BatchID] = []DrainInfo{}
		}
		if _, ok := batchValue[promosDrained[i].BatchID]; !ok {
			batchValue[promosDrained[i].BatchID] = decimal.Zero
		}
		batches[promosDrained[i].BatchID] = append(
			batches[promosDrained[i].BatchID],
			DrainInfo{
				PromotionID:   promosDrained[i].PromotionID,
				TransactionID: promosDrained[i].TransactionID,
				CompletedAt:   promosDrained[i].CompletedAt,
				State:         promosDrained[i].State,
				ErrCode:       promosDrained[i].ErrCode,
				Value:         promosDrained[i].Value,
			},
		)
		batchValue[promosDrained[i].BatchID] = batchValue[promosDrained[i].BatchID].Add(promosDrained[i].Value)
	}

	// for each batch go through and create a custodian drain and add to resp drain
	// add values along the way
	for k := range batches {
		resp = append(resp, CustodianDrain{
			BatchID:           k,
			Custodian:         custodian,
			PromotionsDrained: batches[k],
			Value:             batchValue[k],
		})
	}

	return resp, nil
}

// GetDrainPoll Get the status of the drain poll job
func (pg *Postgres) GetDrainPoll(drainID *uuid.UUID) (*DrainPoll, error) {
	type dbDrainPoll struct {
		ID         *uuid.UUID `db:"batch_id"`
		Completed  bool       `db:"completed"`
		Pending    bool       `db:"pending"`
		Delayed    bool       `db:"delayed"`
		InProgress bool       `db:"inprogress"`
	}
	var (
		drainPoll = new(dbDrainPoll)
		err       error
	)

	statement := `
select
	batch_id,
	bool_and(completed) as completed,
	bool_or(erred) as delayed,
	(not bool_and(completed) and not bool_or(erred)) as inprogress,
	(not bool_or(completed)) as pending
from
	claim_drain
where
	batch_id = $1
group by
	batch_id`

	err = pg.RawDB().Get(drainPoll, statement, drainID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &DrainPoll{
				ID:     drainID,
				Status: "unknown",
			}, nil
		}
		return nil, err
	}

	if drainPoll.Completed {
		return &DrainPoll{
			ID:     drainID,
			Status: "complete",
		}, nil
	}

	if drainPoll.Delayed {
		return &DrainPoll{
			ID:     drainID,
			Status: "delayed",
		}, nil
	}

	if drainPoll.Pending {
		return &DrainPoll{
			ID:     drainID,
			Status: "pending",
		}, nil
	}

	if drainPoll.InProgress {
		return &DrainPoll{
			ID:     drainID,
			Status: "in_progress",
		}, nil
	}

	return &DrainPoll{
		ID:     drainID,
		Status: "unknown",
	}, nil
}

// GetClaimSummary aggregates the values of a single wallet's claims
func (pg *Postgres) GetClaimSummary(walletID uuid.UUID, grantType string) (*ClaimSummary, error) {
	statement := `
select
	max(claims.created_at) as "last_claim",
	sum(claims.approximate_value - claims.bonus) as earnings,
	sum(claims.approximate_value - claims.bonus) as amount,
	promos.promotion_type as type
from claims, (
	select
		id,
		promotion_type
	from promotions
	where promotion_type = $2
	and id not in (select unnest($3::uuid[]))
) as promos
where claims.wallet_id = $1
	and (claims.redeemed = true or claims.legacy_claimed = true)
	and claims.promotion_id = promos.id
group by promos.promotion_type;`

	braveTransferUUIDs, _ := toUUIDs(strings.Split(os.Getenv("BRAVE_TRANSFER_PROMOTION_IDS"), " ")...)

	summaries := []ClaimSummary{}
	err := pg.RawDB().Select(&summaries, statement, walletID, grantType, pq.Array(braveTransferUUIDs))
	if err != nil {
		return nil, err
	}
	if len(summaries) > 0 {
		return &summaries[0], nil
	}

	return nil, nil
}

// GetClaimByWalletAndPromotion gets whether a wallet has a claimed grants
// with the given promotion and returns the grant if so
func (pg *Postgres) GetClaimByWalletAndPromotion(
	wallet *walletutils.Info,
	promotion *Promotion,
) (*Claim, error) {
	query := `
SELECT
  *
FROM claims
	WHERE wallet_id = $1
	AND promotion_id = $2
	AND (legacy_claimed or redeemed)
ORDER BY created_at DESC
`
	claims := []Claim{}
	err := pg.RawDB().Select(&claims, query, wallet.ID, promotion.ID)
	if err != nil {
		return nil, err
	}
	if len(claims) > 0 {
		return &claims[0], nil
	}

	return nil, nil
}

// RunNextBatchPaymentsJob to sign claim credentials if there is a claim waiting, returning true if a job was attempted
func (pg *Postgres) RunNextBatchPaymentsJob(ctx context.Context, worker BatchTransferWorker) (bool, error) {
	// setup a logger
	logger := logging.Logger(ctx, "promotion.RunNextBatchPaymentsJob")
	// create a tx
	tx, err := pg.RawDB().Beginx()
	attempted := false
	if err != nil {
		return attempted, err
	}
	defer pg.RollbackTx(tx)

	// first get a lock on the batch id,
	// only for batches that are all "pending" and have transaction_ids

	statement := `
		select
			cd.batch_id
		from
			claim_drain cd
			join wallets w on w.id=cd.wallet_id
		where
			cd.erred = false and
			w.user_deposit_account_provider = 'bitflyer'
		group by
			cd.batch_id
		having bool_and(transaction_id is not null) = true 
		   and bool_and(cd.status = 'prepared') = true
		limit 1
`
	var batchID = new(uuid.UUID)

	err = tx.Get(batchID, statement)
	if err != nil {
		// no claims to process
		if errors.Is(err, sql.ErrNoRows) {
			return attempted, nil
		}
		return attempted, fmt.Errorf("batch payment job: sql error %w", err)
	}
	attempted = true

	// put a lock on the batch so it is not picked up
	query := "SELECT pg_advisory_xact_lock(hashtext($1))"
	_, err = tx.ExecContext(ctx, query, batchID.String())
	if err != nil {
		return false, fmt.Errorf("failed to acquire tx lock for batch id %s: %w", batchID.String(), err)
	}

	// perform submit against payments API
	err = worker.SubmitBatchTransfer(ctx, batchID)
	if err != nil {

		logger.Error().Err(err).Msg("run next batch payments: failed to submit batch transfers")
		status, errCode, _ := errToDrainCode(err)
		sentry.CaptureException(fmt.Errorf("errCode: %s - %w", errCode, err))
		countClaimDrainStatus.With(prometheus.Labels{"custodian": "bitflyer", "status": "failed"}).Inc()

		stmt := "update claim_drain set erred = true, errcode = $1, status = $2 where batch_id = $3"
		if _, err := tx.Exec(stmt, errCode, status, batchID); err != nil {
			return attempted, err
		}

		if err := tx.Commit(); err != nil {
			return attempted, err
		}

		return attempted, err
	}

	_, err = tx.Exec(`
		update claim_drain set status = 'submitted'	
			where batch_id = $1 and 
			      erred = false and 
			      transaction_id is not null`, batchID)
	if err != nil {
		return attempted, err
	}

	err = tx.Commit()
	if err != nil {
		return attempted, err
	}

	countClaimDrainStatus.With(prometheus.Labels{"custodian": "bitflyer", "status": "complete"}).Inc()

	return attempted, nil
}

// RunNextClaimJob to sign claim credentials if there is a claim waiting, returning true if a job was attempted
func (pg *Postgres) RunNextClaimJob(ctx context.Context, worker ClaimWorker) (bool, error) {
	tx, err := pg.RawDB().Beginx()
	attempted := false
	if err != nil {
		return attempted, err
	}
	defer pg.RollbackTx(tx)

	type SigningJob struct {
		Issuer
		ClaimID      uuid.UUID                 `db:"claim_id"`
		BlindedCreds jsonutils.JSONStringArray `db:"blinded_creds"`
	}

	statement := `
select
	issuers.*,
	claim_cred.claim_id,
	claim_cred.blinded_creds
from
	(select *
	from claim_creds
	where batch_proof is null
	for update skip locked
	limit 1
) claim_cred
inner join issuers
on claim_cred.issuer_id = issuers.id`

	jobs := []SigningJob{}
	err = tx.Select(&jobs, statement)
	if err != nil {
		return attempted, err
	}

	if len(jobs) != 1 {
		return attempted, nil
	}

	job := jobs[0]

	attempted = true
	creds, err := worker.SignClaimCreds(ctx, job.ClaimID, job.Issuer, job.BlindedCreds)
	if err != nil {
		// FIXME certain errors are not recoverable
		return attempted, err
	}

	_, err = tx.Exec(`update claim_creds set signed_creds = $1, batch_proof = $2, public_key = $3, updated_at = now() where claim_id = $4`,
		creds.SignedCreds, creds.BatchProof, creds.PublicKey, creds.ID)
	if err != nil {
		return attempted, err
	}

	err = tx.Commit()
	if err != nil {
		return attempted, err
	}

	return attempted, nil
}

// InsertSuggestion inserts a transaction awaiting validation
func (pg *Postgres) InsertSuggestion(credentials []cbr.CredentialRedemption, suggestionText string, suggestionEvent []byte) error {
	credentialsJSON, err := json.Marshal(credentials)
	if err != nil {
		return err
	}

	statement := `
	insert into suggestion_drain (credentials, suggestion_text, suggestion_event)
	values ($1, $2, $3)
	returning *`
	_, err = pg.RawDB().Exec(statement, credentialsJSON, suggestionText, suggestionEvent)
	if err != nil {
		return err
	}

	return nil
}

// SuggestionJob - representation of a suggestion job
type SuggestionJob struct {
	ID              uuid.UUID `db:"id"`
	Credentials     string    `db:"credentials"`
	SuggestionText  string    `db:"suggestion_text"`
	SuggestionEvent []byte    `db:"suggestion_event"`
	Erred           bool      `db:"erred"`
	ErrCode         *string   `db:"errcode"`
	CreatedAt       time.Time `db:"created_at"`
}

// RunNextSuggestionJob to process a suggestion if there is one waiting
func (pg *Postgres) RunNextSuggestionJob(ctx context.Context, worker SuggestionWorker) (bool, error) {

	tx, err := pg.RawDB().Beginx()
	attempted := false
	if err != nil {
		return attempted, err
	}
	defer pg.RollbackTx(tx)

	// FIXME

	statement := `
select *
from suggestion_drain
where not erred
for update skip locked
limit 1`

	jobs := []SuggestionJob{}
	err = tx.Select(&jobs, statement)
	if err != nil {
		return attempted, err
	}

	if len(jobs) != 1 {
		return attempted, nil
	}

	job := jobs[0]
	attempted = true

	var credentials []cbr.CredentialRedemption
	err = json.Unmarshal([]byte(job.Credentials), &credentials)
	if err != nil {
		return attempted, err
	}

	// if the error code is "cbr_dup_redeem" we can skip the redeem credentials on drain
	// as we are reprocessing a failed job that failed due to duplicate cbr redeem
	if job.ErrCode != nil && *job.ErrCode == "cbr_dup_redeem" {
		ctx = context.WithValue(ctx, appctx.SkipRedeemCredentialsCTXKey, true)
	}

	if !worker.IsPaused() {
		err = worker.RedeemAndCreateSuggestionEvent(ctx, credentials, job.SuggestionText, job.SuggestionEvent)
		if err != nil {
			if strings.Contains(err.Error(), "expired") {
				// set flag to stop this worker from running again
				worker.PauseWorker(time.Now().Add(30 * time.Minute))
			}

			// add jobID and inform sentry about this error
			err = fmt.Errorf("failed to redeem and create suggestion event for jobID %s: %w", job.ID, err)
			sentry.CaptureException(err)

			// update suggestion drain as erred

			_, errCode, _ := errToDrainCode(err)

			stmt := "update suggestion_drain set erred = true, errcode = $1 where id = $2"
			if _, err := tx.Exec(stmt, errCode, job.ID); err != nil {
				return attempted, fmt.Errorf("failed to update errored suggestion job: jobID %s: %w", job.ID, err)
			}

			if err := tx.Commit(); err != nil {
				return attempted, fmt.Errorf("failed to commit txn for suggestion job: jobID %s: %w", job.ID, err)
			}

			return attempted, err
		}

		stmt := "delete from suggestion_drain where id = $1"
		_, err = tx.Exec(stmt, job.ID)
		if err != nil {
			return attempted, err
		}

		err = tx.Commit()
		if err != nil {
			return attempted, err
		}
	}

	return attempted, nil
}

// This code can be deleted once https://github.com/brave-intl/bat-go/issues/263 is addressed.

// GetOrder queries the database and returns an order
func (pg *Postgres) GetOrder(orderID uuid.UUID) (*Order, error) {
	statement := "SELECT * FROM orders WHERE id = $1"
	order := Order{}
	err := pg.RawDB().Get(&order, statement, orderID)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to get order : %w", err)
	}

	foundOrderItems := []OrderItem{}
	statement = "SELECT * FROM order_items WHERE order_id = $1"
	err = pg.RawDB().Select(&foundOrderItems, statement, orderID)

	order.Items = foundOrderItems
	if err != nil {
		return nil, err
	}

	return &order, nil
}

// SetMintDrainPromotionTotal - set the total number of redemptions for this drain job
func (pg *Postgres) SetMintDrainPromotionTotal(ctx context.Context, walletID, promotionID uuid.UUID, total decimal.Decimal) error {

	statement := `
update mint_drain_promotion set total = $1, done = true where
mint_drain_id=(select id from mint_drain where wallet_id=$2) and
promotion_id=$3`

	_, err := pg.Exec(statement, total, walletID, promotionID)
	if err != nil {
		return err
	}

	return nil
}

// EnqueueMintDrainJob - enqueue a mint drain job in "pending" status
func (pg *Postgres) EnqueueMintDrainJob(ctx context.Context, walletID uuid.UUID, promotionIDs ...uuid.UUID) error {
	tx, err := pg.RawDB().Beginx()
	if err != nil {
		return err
	}
	defer pg.RollbackTx(tx)

	var mintDrainJob = MintDrainJob{}

	statement := `
	insert into mint_drain (wallet_id)
	values ($1)
	returning *`
	err = tx.GetContext(ctx, &mintDrainJob, statement, walletID)
	if err != nil {
		return err
	}

	for _, id := range promotionIDs {
		_, err = tx.Exec(`
			insert into mint_drain_promotion
				(mint_drain_id, promotion_id)
			values
				($1, $2)`, mintDrainJob.ID, id)
		if err != nil {
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

// DrainClaim by marking the claim as drained and inserting a new drain entry
func (pg *Postgres) DrainClaim(batchID *uuid.UUID, claim *Claim, credentials []cbr.CredentialRedemption, wallet *walletutils.Info, total decimal.Decimal, codedErr errorutils.DrainCodified) error {
	tx, err := pg.RawDB().Beginx()
	if err != nil {
		return err
	}
	defer pg.RollbackTx(tx)

	err = pg.txDrainClaim(tx, batchID, claim, credentials, wallet, total, codedErr)
	if err != nil {
		return fmt.Errorf("drain claim: error for claimID %s: %w", claim.ID, err)
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

// DrainClaims marks all drain claim as drained and inserts a new drain entry
func (pg *Postgres) DrainClaims(drainClaims []DrainClaim) error {
	tx, err := pg.RawDB().Beginx()
	if err != nil {
		return fmt.Errorf("insert batch drain claim: error could not begin tx: %w", err)
	}
	defer pg.RollbackTx(tx)

	for _, d := range drainClaims {
		err = pg.txDrainClaim(tx, d.BatchID, d.Claim, d.Credentials, d.Wallet, d.Total, d.CodedErr)
		if err != nil {
			return fmt.Errorf("insert batch drain claim: error could not insert drain claim for claimID %s: %w",
				d.Claim.ID, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("insert batch drain claim: error could not commit drain claims: %w", err)
	}

	return nil
}

func (pg *Postgres) txDrainClaim(tx *sqlx.Tx, batchID *uuid.UUID, claim *Claim, credentials []cbr.CredentialRedemption,
	wallet *walletutils.Info, total decimal.Decimal, codedErr errorutils.DrainCodified) error {

	credentialsJSON, err := json.Marshal(credentials)
	if err != nil {
		return err
	}

	var claimID *uuid.UUID
	// if the claim is not nil, we should set it to drained, as we are in drained state
	// this often happens when the wallet is mismatched
	if claim != nil {
		_, err = tx.Exec(`update claims set drained = true, drained_at = now() where id = $1 and not drained`, claim.ID)
		if err != nil {
			return fmt.Errorf("failed to set claim as drained: %w", err)
		}
		claimID = &claim.ID
	} else {
		claimID = nil
	}

	var claimDrain = DrainJob{}

	if codedErr == nil {
		statement := `
		insert into claim_drain (credentials, wallet_id, total, batch_id, claim_id, deposit_destination, updated_at)
		values ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
		returning *`
		err = tx.Get(&claimDrain, statement, credentialsJSON, wallet.ID, total, batchID, claim.ID, &wallet.UserDepositDestination)
		if err != nil {
			return err
		}
	} else {
		code, _ := codedErr.DrainCode()

		// insert errored claim drain item
		statement := `
		insert into claim_drain (credentials, wallet_id, total, batch_id, claim_id, deposit_destination, erred, errcode, updated_at)
		values ($1, $2, $3, $4, $5, $6, true, $7, CURRENT_TIMESTAMP)
		returning *`
		err = tx.Get(&claimDrain, statement, credentialsJSON, wallet.ID, total,
			batchID, claimID, &wallet.UserDepositDestination, code)
		if err != nil {
			return fmt.Errorf("failed to insert erred drain job: %w", err)
		}
	}

	return nil
}

// errToDrainCode - given a drain related processing error, generate a code and retriable flag
func errToDrainCode(err error) (string, string, bool) {
	var (
		status    string
		errCode   string
		retriable bool
	)

	if err == nil {
		return "", "", false
	}

	status = "failed"

	var eb *errorutils.ErrorBundle
	if errors.As(err, &eb) {
		// if this is an error bundle, check the "data" for a codified type
		if c, ok := eb.Data().(errorutils.Codified); ok {
			errCode, retriable = c.DrainCode()
			return status, strings.ToLower(errCode), retriable
		} else if c, ok := eb.Data().(errorutils.DrainCodified); ok {
			errCode, retriable = c.DrainCode()
			return status, strings.ToLower(errCode), retriable
		}
	}

	// possible protocol errors
	if errors.Is(err, errorutils.ErrMarshalTransferRequest) {
		errCode = "marshal_transfer"
	} else if errors.Is(err, errorutils.ErrCreateTransferRequest) {
		errCode = "create_transfer"
	} else if errors.Is(err, errorutils.ErrSignTransferRequest) {
		errCode = "sign_transfer"
	} else if errors.Is(err, errorutils.ErrFailedClientRequest) {
		errCode = "failed_client"
		retriable = true
	} else if errors.Is(err, errorutils.ErrFailedBodyRead) {
		errCode = "failed_response_body"
		retriable = true
	} else if errors.Is(err, errorutils.ErrFailedBodyUnmarshal) {
		errCode = "failed_response_unmarshal"
		retriable = true
	} else if errors.Is(err, errReputationServiceFailure) {
		errCode = "reputation-service-failure"
		retriable = true
	} else if errors.Is(err, errWalletNotReputable) {
		errCode = "reputation-failed"
		status = "reputation-failed"
		retriable = false
	} else if errors.Is(err, errWalletDrainLimitExceeded) {
		errCode = "exceeded-withdrawal-limit"
		status = "exceeded-withdrawal-limit"
		retriable = false
	} else {
		errCode = "unknown"
		var bfe *clients.BitflyerError
		if errors.As(err, &bfe) {
			// possible wallet provider specific errors
			if len(bfe.ErrorIDs) > 0 {
				errCode = fmt.Sprintf("bitflyer_%s", bfe.ErrorIDs[0])
			}
		}
	}
	return status, strings.ToLower(errCode), retriable
}

// DrainJob - definition of a drain job
type DrainJob struct {
	ID                 uuid.UUID       `db:"id"`
	ClaimID            *uuid.UUID      `db:"claim_id"`
	Credentials        string          `db:"credentials"`
	WalletID           uuid.UUID       `db:"wallet_id"`
	Total              decimal.Decimal `db:"total"`
	TransactionID      *string         `db:"transaction_id"`
	Erred              bool            `db:"erred"`
	ErrCode            *string         `db:"errcode"`
	Status             *string         `db:"status"`
	BatchID            *uuid.UUID      `db:"batch_id"`
	Completed          bool            `db:"completed"`
	CompletedAt        pq.NullTime     `db:"completed_at"`
	UpdatedAt          pq.NullTime     `db:"updated_at"`
	DepositDestination *string         `db:"deposit_destination"`
}

var txStatusToStatus = map[string]string{
	"bitflyer-consolidate": "prepared",
	txnStatusGeminiPending: txnStatusGeminiPending,
}

// RunNextDrainJob to process deposits if there is one waiting
func (pg *Postgres) RunNextDrainJob(ctx context.Context, worker DrainWorker) (bool, error) {

	// setup a logger
	logger := logging.Logger(ctx, "promotion.RunNextDrainJob")

	tx, err := pg.RawDB().Beginx()
	attempted := false
	if err != nil {
		return attempted, err
	}
	defer pg.RollbackTx(tx)

	statement := `
select *
from claim_drain
where not erred and transaction_id is null
and (status is null or status not in ('complete', 'reputation-failed', 'failed', 'prepared', 'gemini-pending', 'submitted'))
for update skip locked
limit 1`

	jobs := []DrainJob{}
	err = tx.Select(&jobs, statement)
	if err != nil {
		return attempted, err
	}

	if len(jobs) != 1 {
		return attempted, nil
	}

	job := jobs[0]
	attempted = true

	// set job status to initialized
	_, err = tx.Exec(`
		update claim_drain set
			status = 'initialized'
		where id = $1`, job.ID)
	if err != nil {
		return attempted, err
	}

	var credentials []cbr.CredentialRedemption
	err = json.Unmarshal([]byte(job.Credentials), &credentials)
	if err != nil {
		return attempted, err
	}

	if job.Status != nil && (*job.Status == "retry-bypass-cbr" || *job.Status == "manual-retry") {
		ctx = context.WithValue(ctx, appctx.SkipRedeemCredentialsCTXKey, true)
	}

	txn, err := worker.RedeemAndTransferFunds(ctx, credentials, job)
	if err != nil || txn == nil {
		// log the error from redeem and transfer
		logger.Error().Err(err).
			Interface("claim_drain_id", job.ID).
			Msg("failed to redeem and transfer funds")
		// do not need to capture wallet is not reputable
		if !errors.Is(err, errWalletNotReputable) &&
			!errors.Is(err, errWalletDrainLimitExceeded) &&
			!errors.Is(err, cbr.ErrDupRedeem) {
			// do not sentry log not reputable or drain limit exceeded, or duplicate redemption
			sentry.CaptureException(err)
		}

		// record as error (retriable or not)
		status, errCode, _ := errToDrainCode(err)
		if _, err := tx.Exec(`
				update claim_drain set
					erred = true,
					errcode=$1,
					status=$3
				where id = $2`, errCode, job.ID, status); err == nil {
			_ = tx.Commit()
		}
		return attempted, err
	}

	// if the txn cannot be set as complete immediately then get the status code and update the job
	if status, ok := txStatusToStatus[txn.Status]; ok {
		_, err = tx.Exec(`
			update claim_drain set
				transaction_id = $1,
				status = $2
			where id = $3`, txn.ID, status, job.ID)
		if err != nil {
			return attempted, err
		}
	} else {
		countClaimDrainStatus.With(prometheus.Labels{"custodian": "uphold", "status": "complete"}).Inc()
		_, err = tx.Exec(`
			update claim_drain set
				transaction_id = $1,
				completed = true,
				completed_at = now(),
				status = 'complete'
			where id = $2`, txn.ID, job.ID)
		if err != nil {
			return attempted, err
		}
	}

	err = tx.Commit()
	if err != nil {
		return attempted, err
	}

	return attempted, nil
}

// RunNextDrainRetryJob - toggles failed drain jobs to be reprocessed if eligible
func (pg *Postgres) RunNextDrainRetryJob(ctx context.Context, worker DrainRetryWorker) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			walletID, err := worker.FetchAdminAttestationWalletID(ctx)
			if err != nil {
				return fmt.Errorf("drain retry job: failed to retrieve walletID: %w", err)
			}
			query := `
					UPDATE claim_drain
					SET erred = FALSE, status = 'retry-bypass-cbr'
					WHERE wallet_id = $1 AND erred = TRUE AND errcode = 'reputation-failed' AND status = 'reputation-failed'
				`
			result, err := pg.ExecContext(ctx, query, walletID.String())
			if err != nil {
				err = fmt.Errorf("drain retry job: failed to update drain job for walletID %s: %w ", walletID, err)
				logging.FromContext(ctx).Error().Err(err).Msg("")
				sentry.CaptureException(err)
			} else {
				rowsAffected, err := result.RowsAffected()
				if err != nil {
					err = fmt.Errorf("drain retry job: failed to get rows affected for walletID %s: %w ", walletID, err)
					logging.FromContext(ctx).Error().Err(err).Msg("")
					sentry.CaptureException(err)
				}
				if rowsAffected > 0 {
					logging.FromContext(ctx).Info().
						Msgf("drain retry job: successfully updated drain job for walletID %s", walletID)
				}
			}
		}
	}
}

// MintDrainJob - Job structure for the mint_drain queue
type MintDrainJob struct {
	ID       uuid.UUID       `db:"id"`
	WalletID uuid.UUID       `db:"wallet_id"`
	Total    decimal.Decimal `db:"total"`
	Done     bool            `db:"done"`
	Status   string          `db:"status"`
	Erred    bool            `db:"erred"`
}

// MintDrainPromotion - a list of promotions associated with a mint job
type MintDrainPromotion struct {
	MintJobID   uuid.UUID       `db:"mint_drain_id"`
	PromotionID uuid.UUID       `db:"promotion_id"`
	Done        bool            `db:"done"`
	Total       decimal.Decimal `db:"total"`
}

const (
	// MintDrainJobPending - pending status for the mint_drain job
	MintDrainJobPending = "pending"
	// MintDrainJobFailed - failed status for the mint_drain job
	MintDrainJobFailed = "failed"
	// MintDrainJobComplete - complete status for the mint_drain job
	MintDrainJobComplete = "complete"
)

// RunNextMintDrainJob to process mints vg
func (pg *Postgres) RunNextMintDrainJob(ctx context.Context, worker MintWorker) (bool, error) {

	// setup a logger
	logger := logging.Logger(ctx, "promotion.RunNextMintDrainJob")

	// get and parse the correct transfer promotion id to create claims on
	braveTransferPromotionIDs, ok := ctx.Value(appctx.BraveTransferPromotionIDCTXKey).([]string)
	if !ok {
		logger.Error().Err(errMissingTransferPromotion).
			Msg("MintJob: missing transfer promotion id")
		return false, errMissingTransferPromotion
	}

	tx, err := pg.RawDB().Beginx()
	attempted := false
	if err != nil {
		return attempted, err
	}
	defer pg.RollbackTx(tx)

	// get the mint job. only the ones that have finished all the promotion totals
	statement := `
select md.*,
	(select sum(mdp.total) from mint_drain_promotion as mdp where mdp.mint_drain_id=md.id) as total,
	(select bool_and(mdp.done) from mint_drain_promotion as mdp where mdp.mint_drain_id=md.id) as done
from mint_drain as md
where not md.erred and md.status = 'pending' and
(select bool_and(done) from mint_drain_promotion where mint_drain_id=md.id)
for update of md skip locked
limit 1;
`

	job := MintDrainJob{}
	err = tx.Get(&job, statement)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return attempted, nil
		}
		return attempted, err
	}
	// are all of the claims associated with all of the promotions drained?

	statement = `
select
	bool_and(c.drained)
from
	mint_drain_promotion mdp
	join claims c
		on (c.promotion_id=mdp.promotion_id)
where
	mdp.mint_drain_id = $1
	and c.wallet_id= $2
`
	var drained bool
	err = tx.Get(&drained, statement, job.ID, job.WalletID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return attempted, nil
		}
		return attempted, err
	}

	if !drained {
		return attempted, nil
	}

	// yes? set status to complete and mint the grants
	promoIDs, err := toUUIDs(braveTransferPromotionIDs...)
	if err != nil {
		// log the error from redeem and transfer
		logger.Error().Err(err).Msg("failed to derive promotion ids from configuration")
		return attempted, err
	}

	attempted = true

	// mint the grant to the wallet's deposit destination
	statement = `
select
	w.user_deposit_destination
from
	wallets w
where
	w.id = $1
`
	var (
		depositDestination string
	)
	err = tx.Get(&depositDestination, statement, job.WalletID)
	if err != nil {
		return attempted, err
	}

	if depositDestination == "" {
		return attempted, errors.New("wallet is not verified")
	}

	depositDestinationUUID, err := uuid.FromString(depositDestination)
	if err != nil {
		return attempted, errors.New("destination invalid wallet id")
	}

	err = worker.MintGrant(ctx, depositDestinationUUID, job.Total, promoIDs...)
	if err != nil {
		// log the error from redeem and transfer
		logger.Error().Err(err).Msg("failed to mint grants")
		if _, err := tx.Exec(`update mint_drain set erred = true where id = $1`, job.ID); err != nil {
			pg.RollbackTx(tx)
		}
		_ = tx.Commit()
		return attempted, err
	}

	if _, err := tx.Exec(`update mint_drain set status = 'complete' where id = $1`, job.ID); err != nil {
		pg.RollbackTx(tx)
	}

	err = tx.Commit()
	if err != nil {
		return attempted, err
	}

	return attempted, nil
}

// UpdateOrder updates the orders status.
//	Status should either be one of pending, paid, fulfilled, or canceled.
func (pg *Postgres) UpdateOrder(orderID uuid.UUID, status string) error {
	result, err := pg.RawDB().Exec(`UPDATE orders set status = $1, updated_at = CURRENT_TIMESTAMP where id = $2`, status, orderID)

	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if rowsAffected == 0 || err != nil {
		return errors.New("no rows updated")
	}

	return nil
}

// CreateTransaction creates a transaction given an orderID, externalTransactionID, currency, and a kind of transaction
func (pg *Postgres) CreateTransaction(orderID uuid.UUID, externalTransactionID string, status string, currency string, kind string, amount decimal.Decimal) (*Transaction, error) {
	tx := pg.RawDB().MustBegin()
	defer pg.RollbackTx(tx)

	var transaction Transaction
	err := tx.Get(&transaction,
		`
			INSERT INTO transactions (order_id, external_transaction_id, status, currency, kind, amount)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING *
	`, orderID, externalTransactionID, status, currency, kind, amount)

	if err != nil {
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return &transaction, nil
}

// GetSumForTransactions returns the calculated sum
func (pg *Postgres) GetSumForTransactions(orderID uuid.UUID) (decimal.Decimal, error) {
	var sum decimal.Decimal

	err := pg.RawDB().Get(&sum, `
		SELECT SUM(amount) as sum
		FROM transactions
		WHERE order_id = $1 AND status = 'completed'
	`, orderID)

	return sum, err
}

// UpdateDrainJobAsRetriable - updates a drain job as retriable
func (pg *Postgres) UpdateDrainJobAsRetriable(ctx context.Context, walletID uuid.UUID) error {
	query := `
				UPDATE claim_drain
				SET erred = FALSE, status = 'manual-retry'
				WHERE wallet_id = $1 AND erred = TRUE AND status IN ('reputation-failed', 'failed') AND transaction_id IS NULL
			`
	result, err := pg.ExecContext(ctx, query, walletID)
	if err != nil {
		return fmt.Errorf("update drain job: failed to exec update for walletID %s: %w", walletID, err)
	}

	affectedRows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update drain job: failed to get affected rows for walletID %s: %w", walletID, err)
	}

	if affectedRows == 0 {
		return fmt.Errorf("update drain job: failed to update row for walletID %s: %w", walletID,
			errorutils.ErrNotFound)
	}

	return nil
}

// RunNextGeminiCheckStatus periodically check the status of gemini claim drain transactions
func (pg *Postgres) RunNextGeminiCheckStatus(ctx context.Context, worker GeminiTxnStatusWorker) (bool, error) {
	tx, err := pg.RawDB().Beginx()
	if err != nil {
		return false, fmt.Errorf("gemini check status job: failed to begin transaction: %w", err)
	}
	defer pg.RollbackTx(tx)

	var drainJob DrainJob
	err = tx.Get(&drainJob, `
									select * from claim_drain 
									where status = $1 and transaction_id is not null 
									  and updated_at < NOW() - interval '10 MINUTES'									
									order by updated_at asc
									for update skip locked limit 1
									    `, txnStatusGeminiPending)
	if err != nil {
		// no drains to process
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("gemini check status job: sql error %w", err)
	}

	settlementTx := custodian.Transaction{
		SettlementID: ptr.String(drainJob.TransactionID),
		Type:         "drain",
		Destination:  ptr.String(drainJob.DepositDestination),
		Channel:      "wallet",
	}
	txRef := gemini.GenerateTxRef(&settlementTx)

	transactionInfo, err := worker.GetGeminiTxnStatus(ctx, txRef)
	if err != nil || transactionInfo == nil {

		// update the erred claim drain so it goes to back of queue
		query := `update claim_drain set status = $1, updated_at = now() where id = $2`
		if _, err := tx.ExecContext(ctx, query, txnStatusGeminiPending, drainJob.ID); err != nil {
			return true, fmt.Errorf("failed to update status for txn %s: %w", *drainJob.TransactionID, err)
		}

		if err := tx.Commit(); err != nil {
			return true, fmt.Errorf("failed to commit update status for txn %s: %w", *drainJob.TransactionID, err)
		}

		return true, fmt.Errorf("failed to get status for txn %s: %w", *drainJob.TransactionID, err)
	}

	switch transactionInfo.Status {
	case "complete":
		query := `update claim_drain set completed = true, completed_at = now(), status = 'complete' where id = $1`
		if _, err := tx.ExecContext(ctx, query, drainJob.ID); err != nil {
			return true, fmt.Errorf("failed to update status for txn %s: %w", *drainJob.TransactionID, err)
		}
	case "pending":
		query := `update claim_drain set status = $1, updated_at = now() where id = $2`
		if _, err := tx.ExecContext(ctx, query, txnStatusGeminiPending, drainJob.ID); err != nil {
			return true, fmt.Errorf("failed to update status for txn %s: %w", *drainJob.TransactionID, err)
		}
	case "failed":
		query := `update claim_drain set status = 'failed', erred = true, errcode = $1 where id = $2`
		if _, err := tx.ExecContext(ctx, query, transactionInfo.Note, drainJob.ID); err != nil {
			return true, fmt.Errorf("failed to update status for txn %s: %w", *drainJob.TransactionID, err)
		}
	default:
		return true, fmt.Errorf("failed to update status for txn %s: unknown status %s",
			*drainJob.TransactionID, transactionInfo.Status)
	}

	err = tx.Commit()
	if err != nil {
		return true, fmt.Errorf("failed to commit update status for txn %s: %w", *drainJob.TransactionID, err)
	}

	if transactionInfo.Status == "complete" || transactionInfo.Status == "failed" {
		countClaimDrainStatus.With(prometheus.Labels{"custodian": "gemini", "status": transactionInfo.Status}).Inc()
	}

	return true, nil
}

func toUUIDs(a ...string) ([]uuid.UUID, error) {
	var (
		b = []uuid.UUID{}
	)
	for _, id := range a {
		v, err := uuid.FromString(id)
		if err != nil {
			return nil, err
		}
		b = append(b, v)
	}
	return b, nil
}

// GetDrainsByBatchID - get the drain by the batch id
func (pg *Postgres) GetDrainsByBatchID(ctx context.Context, batchID *uuid.UUID) ([]DrainTransfer, error) {
	resp := []DrainTransfer{}
	// get the linked wallet info
	stmt := `
select
	transaction_id, total, deposit_destination
from
	claim_drain
where
	batch_id = $1
`
	if err := pg.RawDB().Select(&resp, stmt, batchID); err != nil {
		return nil, err
	}

	return resp, nil
}

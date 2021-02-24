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

	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/jsonutils"
	"github.com/brave-intl/bat-go/utils/logging"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/getsentry/sentry-go"
	"github.com/lib/pq"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var desktopPlatforms = [...]string{"linux", "osx", "windows"}

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

// Datastore abstracts over the underlying datastore
type Datastore interface {
	grantserver.Datastore
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
	DrainClaim(drainID *uuid.UUID, claim *Claim, credentials []cbr.CredentialRedemption, wallet *walletutils.Info, total decimal.Decimal) error
	// RunNextDrainJob to process deposits if there is one waiting
	RunNextDrainJob(ctx context.Context, worker DrainWorker) (bool, error)

	// EnqueueMintDrainJob - enqueue a mint drain job in "pending" status
	EnqueueMintDrainJob(ctx context.Context, walletID uuid.UUID, promotionIDs ...uuid.UUID) error

	// SetMintDrainPromotionTotal - set the per promotion total for the mint drain
	SetMintDrainPromotionTotal(ctx context.Context, walletID, promotionID uuid.UUID, total decimal.Decimal) error

	// RunNextMintDrainJob to create new grants from the mint queue
	RunNextMintDrainJob(ctx context.Context, worker MintWorker) (bool, error)

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
}

// ReadOnlyDatastore includes all database methods that can be made with a read only db connection
type ReadOnlyDatastore interface {
	grantserver.Datastore
	// GetPreClaim is used to fetch a "pre-registered" claim for a particular wallet
	GetPreClaim(promotionID uuid.UUID, walletID string) (*Claim, error)
	// GetAvailablePromotionsForWallet returns the list of available promotions for the wallet
	GetAvailablePromotionsForWallet(wallet *walletutils.Info, platform string) ([]Promotion, error)
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
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	grantserver.Postgres
}

// NewDB creates a new Postgres Datastore
func NewDB(databaseURL string, performMigration bool, dbStatsPrefix ...string) (Datastore, error) {
	pg, err := grantserver.NewPostgres(databaseURL, performMigration, dbStatsPrefix...)
	if pg != nil {
		return &DatastoreWithPrometheus{
			base: &Postgres{*pg}, instanceName: "promotion_datastore",
		}, err
	}
	return nil, err
}

// NewRODB creates a new Postgres RO Datastore
func NewRODB(databaseURL string, performMigration bool, dbStatsPrefix ...string) (ReadOnlyDatastore, error) {
	pg, err := grantserver.NewPostgres(databaseURL, performMigration, dbStatsPrefix...)
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
	pg, err := NewDB("", true, "promotion_db")
	if err != nil {
		sentry.CaptureException(err)
		log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
	}
	roDB := os.Getenv("RO_DATABASE_URL")
	if len(roDB) > 0 {
		roPg, err = NewRODB(roDB, false, "promotion_read_only_db")
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
	statement := "select * from promotions where id = $1"
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
		return nil, errors.New("Unexpected number of issuers returned")
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
		set redeemed = true
		where promotion_id = $1 and wallet_id = $2 and not redeemed
		returning *`
		err = tx.Select(&claims, statement, promotion.ID, wallet.ID)
	} else {
		statement := `
		insert into claims (promotion_id, wallet_id, approximate_value, redeemed)
		values ($1, $2, $3, true)
		returning *`
		err = tx.Select(&claims, statement, promotion.ID, wallet.ID, promotion.ApproximateValue)
	}

	if err != nil {
		return nil, err
	} else if len(claims) != 1 {
		return nil, fmt.Errorf("Incorrect number of claims updated / inserted: %d", len(claims))
	}
	claim := claims[0]

	// This will error if user has already claimed due to uniqueness constraint
	_, err = tx.Exec(`insert into claim_creds (issuer_id, claim_id, blinded_creds) values ($1, $2, $3)`, issuer.ID, claim.ID, blindedCredsJSON)
	if err != nil {
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return &claim, nil
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
				select
					promotions.*,
					array_to_json(array_remove(array_agg(issuers.public_key), null)) as public_keys
				from
				promotions left join issuers on promotions.id = issuers.promotion_id
				where ( promotions.platform = '' or promotions.platform = $2)
				group by promotions.id
			) promos left join (
				select * from claims where claims.wallet_id = $1
			) wallet_claims on promos.id = wallet_claims.promotion_id
		where
			wallet_claims.redeemed is distinct from true and (
				wallet_claims.legacy_claimed is true or (
					promos.created_at > NOW() - INTERVAL '3 months' and promos.active and (
						( promos.promotion_type = 'ugp' and promos.remaining_grants > 0 ) or
						( promos.promotion_type = 'ads' and wallet_claims.id is not null )
					)
				)
			)
		order by promos.created_at;`

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
	_, err := pg.RawDB().Exec(`update claim_creds set signed_creds = $1, batch_proof = $2, public_key = $3 where claim_id = $4`, creds.SignedCreds, creds.BatchProof, creds.PublicKey, creds.ID)
	return err
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

	_, err = tx.Exec(`update claim_creds set signed_creds = $1, batch_proof = $2, public_key = $3 where claim_id = $4`, creds.SignedCreds, creds.BatchProof, creds.PublicKey, creds.ID)
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

// RunNextSuggestionJob to process a suggestion if there is one waiting
func (pg *Postgres) RunNextSuggestionJob(ctx context.Context, worker SuggestionWorker) (bool, error) {

	tx, err := pg.RawDB().Beginx()
	attempted := false
	if err != nil {
		return attempted, err
	}
	defer pg.RollbackTx(tx)

	// FIXME
	type SuggestionJob struct {
		ID              uuid.UUID `db:"id"`
		Credentials     string    `db:"credentials"`
		SuggestionText  string    `db:"suggestion_text"`
		SuggestionEvent []byte    `db:"suggestion_event"`
		Erred           bool      `db:"erred"`
		ErrCode         *string   `db:"errcode"`
	}

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

	if !worker.IsPaused() {
		err = worker.RedeemAndCreateSuggestionEvent(ctx, credentials, job.SuggestionText, job.SuggestionEvent)
		if err != nil {
			if strings.Contains(err.Error(), "expired") {
				// set flag to stop this worker from running again
				worker.PauseWorker(time.Now().Add(30 * time.Minute))
			}
			errCode, retriable := errToDrainCode(err)

			if !retriable {
				if _, err = tx.Exec(
					`update suggestion_drain set erred = true, errcode=$1 where id = $2`, errCode, job.ID); err == nil {
					err = tx.Commit()
				}
				return attempted, err
			}
		}

		_, err = tx.Exec(`delete from suggestion_drain where id = $1`, job.ID)
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
		return nil, fmt.Errorf("Failed to get order : %w", err)
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
func (pg *Postgres) DrainClaim(drainPollID *uuid.UUID, claim *Claim, credentials []cbr.CredentialRedemption, wallet *walletutils.Info, total decimal.Decimal) error {
	credentialsJSON, err := json.Marshal(credentials)
	if err != nil {
		return err
	}

	tx, err := pg.RawDB().Beginx()
	if err != nil {
		return err
	}
	defer pg.RollbackTx(tx)

	_, err = tx.Exec(`update claims set drained = true where id = $1 and not drained`, claim.ID)
	if err != nil {
		return err
	}

	var claimDrain = DrainJob{}

	statement := `
	insert into claim_drain (credentials, wallet_id, total, batch_id)
	values ($1, $2, $3, $4)
	returning *`
	err = tx.Get(&claimDrain, statement, credentialsJSON, wallet.ID, total, drainPollID)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

// errToDrainCode - given a drain related processing error, generate a code and retriable flag
func errToDrainCode(err error) (string, bool) {
	var (
		errCode   string
		retriable bool
	)
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
	} else {
		if codedErr, ok := err.(uphold.Coded); ok {
			// possible wallet provider specific errors
			errCode = codedErr.GetCode()
		} else {
			errCode = "unknown"
		}
	}
	return errCode, retriable
}

// DrainJob - definition of a drain job
type DrainJob struct {
	ID            uuid.UUID       `db:"id"`
	Credentials   string          `db:"credentials"`
	WalletID      uuid.UUID       `db:"wallet_id"`
	Total         decimal.Decimal `db:"total"`
	TransactionID *string         `db:"transaction_id"`
	Erred         bool            `db:"erred"`
	ErrCode       *string         `db:"errcode"`
	BatchID       *uuid.UUID      `db:"batch_id"`
	Completed     bool            `db:"completed"`
	CompletedAt   *time.Time      `db:"completed_at"`
}

// RunNextDrainJob to process deposits if there is one waiting
func (pg *Postgres) RunNextDrainJob(ctx context.Context, worker DrainWorker) (bool, error) {

	// setup a logger
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}

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

	var credentials []cbr.CredentialRedemption
	err = json.Unmarshal([]byte(job.Credentials), &credentials)
	if err != nil {
		return attempted, err
	}

	txn, err := worker.RedeemAndTransferFunds(ctx, credentials, job.WalletID, job.Total)
	if err != nil || txn == nil {
		// log the error from redeem and transfer
		logger.Error().Err(err).Msg("failed to redeem and transfer funds")
		errCode, retriable := errToDrainCode(err)

		if !retriable {
			if _, err := tx.Exec(`update claim_drain set erred = true, errcode=$1 where id = $2`, errCode, job.ID); err != nil {
				pg.RollbackTx(tx)
			}
			_ = tx.Commit()
		}
		return attempted, err
	}

	_, err = tx.Exec(`update claim_drain set transaction_id = $1, completed = true, completed_at = now() where id = $2`, txn.ID, job.ID)
	if err != nil {
		return attempted, err
	}

	err = tx.Commit()
	if err != nil {
		return attempted, err
	}

	return attempted, nil
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
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}

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
	user_deposit_destination
from
	wallets
where
	id = $1
`
	var depositDestination string
	err = tx.Get(&depositDestination, statement, job.WalletID)
	if err != nil {
		return attempted, err
	}

	if depositDestination == "" {
		return attempted, errors.New("Wallet is not verified")
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
		return errors.New("No rows updated")
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

package promotion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	"github.com/brave-intl/bat-go/utils/jsonutils"
	"github.com/brave-intl/bat-go/wallet"
	walletservice "github.com/brave-intl/bat-go/wallet/service"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var desktopPlatforms = [...]string{"linux", "osx", "windows"}

// ClobberedCreds holds data of claims that have been clobbered and when they were first reported
type ClobberedCreds struct {
	ID        uuid.UUID `db:"id"`
	CreatedAt time.Time `db:"created_at"`
}

// Datastore abstracts over the underlying datastore
type Datastore interface {
	walletservice.Datastore
	// ActivatePromotion marks a particular promotion as active
	ActivatePromotion(promotion *Promotion) error
	// DeactivatePromotion marks a particular promotion as inactive
	DeactivatePromotion(promotion *Promotion) error
	// ClaimForWallet is used to either create a new claim or convert a preregistered claim for a particular promotion
	ClaimForWallet(promotion *Promotion, issuer *Issuer, wallet *wallet.Info, blindedCreds jsonutils.JSONStringArray, migrate bool) (*Claim, error)
	// CreateClaim is used to "pre-register" an unredeemed claim for a particular wallet
	CreateClaim(promotionID uuid.UUID, walletID string, value decimal.Decimal, bonus decimal.Decimal) (*Claim, error)
	// GetPreClaim is used to fetch a "pre-registered" claim for a particular wallet
	GetPreClaim(promotionID uuid.UUID, walletID string) (*Claim, error)
	// CreatePromotion given the promotion type, initial number of grants and the desired value of those grants
	CreatePromotion(promotionType string, numGrants int, value decimal.Decimal, platform string) (*Promotion, error)
	// GetAvailablePromotionsForWallet returns the list of available promotions for the wallet
	GetAvailablePromotionsForWallet(wallet *wallet.Info, platform string, legacy bool, migrate bool) ([]Promotion, error)
	// GetAvailablePromotions returns the list of available promotions for all wallets
	GetAvailablePromotions(platform string, legacy bool) ([]Promotion, error)
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
	GetClaimByWalletAndPromotion(wallet *wallet.Info, promotionID *Promotion) (*Claim, error)
	// RunNextClaimJob to sign claim credentials if there is a claim waiting
	RunNextClaimJob(ctx context.Context, worker ClaimWorker) (bool, error)
	// InsertSuggestion inserts a transaction awaiting validation
	InsertSuggestion(credentials []cbr.CredentialRedemption, suggestionText string, suggestion []byte) error
	// RunNextSuggestionJob to process a suggestion if there is one waiting
	RunNextSuggestionJob(ctx context.Context, worker SuggestionWorker) (bool, error)
	// InsertClobberedClaims inserts clobbered claim ids into the clobbered_claims table
	InsertClobberedClaims(ctx context.Context, ids []uuid.UUID) error
	// DrainClaim by marking the claim as drained and inserting a new drain entry
	DrainClaim(claim *Claim, credentials []cbr.CredentialRedemption, wallet *wallet.Info, total decimal.Decimal) error
	// RunNextDrainJob to process deposits if there is one waiting
	RunNextDrainJob(ctx context.Context, worker DrainWorker) (bool, error)

	// Remove once this is completed https://github.com/brave-intl/bat-go/issues/263

	// GetOrder by ID
	GetOrder(orderID uuid.UUID) (*Order, error)
	// UpdateOrder updates an order when it has been paid
	UpdateOrder(orderID uuid.UUID, status string) error
	// CreateTransaction creates a transaction
	CreateTransaction(orderID uuid.UUID, externalTransactionID string, status string, currency string, kind string, amount decimal.Decimal) (*Transaction, error)
	// GetSumForTransactions gets a decimal sum of for transactions for an order
	GetSumForTransactions(orderID uuid.UUID) (decimal.Decimal, error)
}

// ReadOnlyDatastore includes all database methods that can be made with a read only db connection
type ReadOnlyDatastore interface {
	walletservice.ReadOnlyDatastore
	// GetPreClaim is used to fetch a "pre-registered" claim for a particular wallet
	GetPreClaim(promotionID uuid.UUID, walletID string) (*Claim, error)
	// GetAvailablePromotionsForWallet returns the list of available promotions for the wallet
	GetAvailablePromotionsForWallet(wallet *wallet.Info, platform string, legacy bool, migrate bool) ([]Promotion, error)
	// GetAvailablePromotions returns the list of available promotions for all wallets
	GetAvailablePromotions(platform string, legacy bool) ([]Promotion, error)
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
	GetClaimByWalletAndPromotion(wallet *wallet.Info, promotionID *Promotion) (*Claim, error)
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	grantserver.Postgres
}

// NewPostgres creates a new Postgres Datastore
func NewPostgres(databaseURL string, performMigration bool, dbStatsPrefix ...string) (*Postgres, error) {
	pg, err := grantserver.NewPostgres(databaseURL, performMigration, dbStatsPrefix...)
	if pg != nil {
		return &Postgres{*pg}, err
	}
	return nil, err
}

// CreatePromotion given the promotion type, initial number of grants and the desired value of those grants
func (pg *Postgres) CreatePromotion(promotionType string, numGrants int, value decimal.Decimal, platform string) (*Promotion, error) {
	statement := `
	insert into promotions (promotion_type, remaining_grants, approximate_value, suggestions_per_grant, platform)
	values ($1, $2, $3, $4, $5)
	returning *`
	promotions := []Promotion{}
	suggestionsPerGrant := value.Div(defaultVoteValue)
	err := pg.DB.Select(&promotions, statement, promotionType, numGrants, value, suggestionsPerGrant, platform)
	if err != nil {
		return nil, err
	}

	return &promotions[0], nil
}

// GetPromotion by ID
func (pg *Postgres) GetPromotion(promotionID uuid.UUID) (*Promotion, error) {
	statement := "select * from promotions where id = $1"
	promotions := []Promotion{}
	err := pg.DB.Select(&promotions, statement, promotionID)
	if err != nil {
		return nil, err
	}

	if len(promotions) > 0 {
		return &promotions[0], nil
	}

	return nil, nil
}

// InsertClobberedClaims inserts clobbered claims to the db
func (pg *Postgres) InsertClobberedClaims(ctx context.Context, ids []uuid.UUID) error {
	tx, err := pg.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer pg.RollbackTx(tx)
	for _, id := range ids {
		_, err = tx.Exec(`INSERT INTO clobbered_claims (id) values ($1) ON CONFLICT DO NOTHING;`, id)
		if err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
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
	_, err := pg.DB.Exec("update promotions set active = $2 where id = $1", promotion.ID, active)
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
	err := pg.DB.Select(&issuers, statement, issuer.PromotionID, issuer.Cohort, issuer.PublicKey)
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
	err := pg.DB.Select(&issuers, statement, promotionID.String(), cohort)
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
	err := pg.DB.Select(&issuers, statement, publicKey)
	if err != nil {
		return nil, err
	}

	if len(issuers) > 0 {
		return &issuers[0], nil
	}

	return nil, nil
}

// CreateClaim is used to "pre-register" an unredeemed claim for a particular wallet
func (pg *Postgres) CreateClaim(promotionID uuid.UUID, walletID string, value decimal.Decimal, bonus decimal.Decimal) (*Claim, error) {
	statement := `
	insert into claims (promotion_id, wallet_id, approximate_value, bonus)
	values ($1, $2, $3, $4)
	returning *`
	claims := []Claim{}
	err := pg.DB.Select(&claims, statement, promotionID, walletID, value, bonus)
	if err != nil {
		return nil, err
	}

	return &claims[0], nil
}

// GetPreClaim is used to fetch a "pre-registered" claim for a particular wallet
func (pg *Postgres) GetPreClaim(promotionID uuid.UUID, walletID string) (*Claim, error) {
	claims := []Claim{}
	err := pg.DB.Select(&claims, "select * from claims where promotion_id = $1 and wallet_id = $2", promotionID.String(), walletID)
	if err != nil {
		return nil, err
	}

	if len(claims) > 0 {
		return &claims[0], nil
	}

	return nil, nil
}

// ClaimForWallet is used to either create a new claim or convert a preregistered claim for a particular promotion
func (pg *Postgres) ClaimForWallet(
	promotion *Promotion,
	issuer *Issuer,
	wallet *wallet.Info,
	blindedCreds jsonutils.JSONStringArray,
	migrate bool,
) (*Claim, error) {
	blindedCredsJSON, err := json.Marshal(blindedCreds)
	if err != nil {
		return nil, err
	}

	tx, err := pg.DB.Beginx()
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
func (pg *Postgres) GetAvailablePromotionsForWallet(wallet *wallet.Info, platform string, legacy bool, migrate bool) ([]Promotion, error) {
	for _, desktopPlatform := range desktopPlatforms {
		if platform == desktopPlatform {
			platform = "desktop"
		}
	}
	statement := fmt.Sprintf(`
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
			(
				%s or promos.active
			) and wallet_claims.redeemed is distinct from true and
			( wallet_claims.legacy_claimed is true or (
				(
					( promos.promotion_type = 'ugp' and promos.remaining_grants > 0 ) or
					( promos.promotion_type = 'ads' and wallet_claims.id is not null )
				) and ( promos.created_at > NOW() - INTERVAL '3 months'))
			)
		order by promos.created_at`, strconv.FormatBool(migrate))

	if legacy {
		statement = `
		select
			promotions.*,
			false as legacy_claimed,
			true as available
		from promotions left join (
			select * from claims where claims.wallet_id = $1
		) wallet_claims on promotions.id = wallet_claims.promotion_id
		where
			promotions.active and wallet_claims.redeemed is distinct from true and
			( promotions.platform = '' or promotions.platform = $2) and
			wallet_claims.legacy_claimed is distinct from true and
			( ( promotions.promotion_type = 'ugp' and promotions.remaining_grants > 0 ) or
				( promotions.promotion_type = 'ads' and wallet_claims.id is not null )
			)
		order by promotions.created_at;`
	}

	promotions := []Promotion{}

	err := pg.DB.Select(&promotions, statement, wallet.ID, platform)
	if err != nil {
		return promotions, err
	}

	return promotions, nil
}

// GetAvailablePromotions returns the list of available promotions for all wallets
func (pg *Postgres) GetAvailablePromotions(platform string, legacy bool) ([]Promotion, error) {
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

	if legacy {
		statement = `
		select
			promotions.*,
			false as legacy_claimed,
			true as available,
			array_to_json(array_remove(array_agg(issuers.public_key), null)) as public_keys
		from
		promotions left join issuers on promotions.id = issuers.promotion_id
		where promotions.promotion_type = 'ugp' and promotions.active and
			promotions.remaining_grants > 0 and
			( promotions.platform = '' or promotions.platform = $1 )
		group by promotions.id
		order by promotions.created_at;`
	}

	promotions := []Promotion{}

	err := pg.DB.Select(&promotions, statement, platform)
	if err != nil {
		return promotions, err
	}

	return promotions, nil
}

// GetClaimCreds returns the claim credentials for a ClaimID
func (pg *Postgres) GetClaimCreds(claimID uuid.UUID) (*ClaimCreds, error) {
	claimCreds := []ClaimCreds{}
	err := pg.DB.Select(&claimCreds, "select * from claim_creds where claim_id = $1", claimID)
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
	_, err := pg.DB.Exec(`update claim_creds set signed_creds = $1, batch_proof = $2, public_key = $3 where claim_id = $4`, creds.SignedCreds, creds.BatchProof, creds.PublicKey, creds.ID)
	return err
}

// GetClaimSummary aggregates the values of a single wallet's claims
func (pg *Postgres) GetClaimSummary(walletID uuid.UUID, grantType string) (*ClaimSummary, error) {
	statement := `
select
	max(claims.created_at) as "last_claim",
	sum(claims.approximate_value - claims.bonus) as earnings,
	promos.promotion_type as type
from claims, (
	select
		id,
		promotion_type
	from promotions
	where promotion_type = $2
) as promos
where claims.wallet_id = $1
	and (claims.redeemed = true or claims.legacy_claimed = true)
	and claims.promotion_id = promos.id
group by promos.promotion_type;`
	summaries := []ClaimSummary{}
	err := pg.DB.Select(&summaries, statement, walletID, grantType)
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
	wallet *wallet.Info,
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
	err := pg.DB.Select(&claims, query, wallet.ID, promotion.ID)
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
	tx, err := pg.DB.Beginx()
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
	_, err = pg.DB.Exec(statement, credentialsJSON, suggestionText, suggestionEvent)
	if err != nil {
		return err
	}

	return nil
}

// RunNextSuggestionJob to process a suggestion if there is one waiting
func (pg *Postgres) RunNextSuggestionJob(ctx context.Context, worker SuggestionWorker) (bool, error) {
	tx, err := pg.DB.Beginx()
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

	err = worker.RedeemAndCreateSuggestionEvent(ctx, credentials, job.SuggestionText, job.SuggestionEvent)
	if err != nil {
		// FIXME only non-retriable errors should set erred
		_, err = tx.Exec(`update suggestion_drain set erred = true where id = $1`, job.ID)
		if err == nil {
			err = tx.Commit()
		}
		return attempted, err
	}

	_, err = tx.Exec(`delete from suggestion_drain where id = $1`, job.ID)
	if err != nil {
		return attempted, err
	}

	err = tx.Commit()
	if err != nil {
		return attempted, err
	}

	return attempted, nil
}

// This code can be deleted once https://github.com/brave-intl/bat-go/issues/263 is addressed.

// GetOrder queries the database and returns an order
func (pg *Postgres) GetOrder(orderID uuid.UUID) (*Order, error) {
	statement := "SELECT * FROM orders WHERE id = $1"
	order := Order{}
	err := pg.DB.Get(&order, statement, orderID)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("Failed to get order : %w", err)
	}

	foundOrderItems := []OrderItem{}
	statement = "SELECT * FROM order_items WHERE order_id = $1"
	err = pg.DB.Select(&foundOrderItems, statement, orderID)

	order.Items = foundOrderItems
	if err != nil {
		return nil, err
	}

	return &order, nil
}

// DrainClaim by marking the claim as drained and inserting a new drain entry
func (pg *Postgres) DrainClaim(claim *Claim, credentials []cbr.CredentialRedemption, wallet *wallet.Info, total decimal.Decimal) error {
	credentialsJSON, err := json.Marshal(credentials)
	if err != nil {
		return err
	}

	tx, err := pg.DB.Beginx()
	if err != nil {
		return err
	}
	defer pg.RollbackTx(tx)

	_, err = tx.Exec(`update claims set drained = true where id = $1 and not drained`, claim.ID)
	if err != nil {
		return err
	}

	statement := `
	insert into claim_drain (credentials, wallet_id, total)
	values ($1, $2, $3)
	returning *`
	_, err = tx.Exec(statement, credentialsJSON, wallet.ID, total)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

// RunNextDrainJob to process deposits if there is one waiting
func (pg *Postgres) RunNextDrainJob(ctx context.Context, worker DrainWorker) (bool, error) {
	tx, err := pg.DB.Beginx()
	attempted := false
	if err != nil {
		return attempted, err
	}
	defer pg.RollbackTx(tx)

	// FIXME maybe useful to later move definition outside of this method scope
	type DrainJob struct {
		ID            uuid.UUID       `db:"id"`
		Credentials   string          `db:"credentials"`
		WalletID      uuid.UUID       `db:"wallet_id"`
		Total         decimal.Decimal `db:"total"`
		TransactionID *string         `db:"transaction_id"`
		Erred         bool            `db:"erred"`
	}

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
		// FIXME only non-retriable errors should set erred
		{
			_, err := tx.Exec(`update claim_drain set erred = true where id = $1`, job.ID)
			if err != nil {
				pg.RollbackTx(tx)
			}
			_ = tx.Commit()
		}
		return attempted, err
	}

	_, err = tx.Exec(`update claim_drain set transaction_id = $1 where id = $2`, txn.ID, job.ID)
	if err != nil {
		return attempted, err
	}

	err = tx.Commit()
	if err != nil {
		return attempted, err
	}

	return attempted, nil
}

// UpdateOrder updates the orders status.
// 	Status should either be one of pending, paid, fulfilled, or canceled.
func (pg *Postgres) UpdateOrder(orderID uuid.UUID, status string) error {
	result, err := pg.DB.Exec(`UPDATE orders set status = $1, updated_at = CURRENT_TIMESTAMP where id = $2`, status, orderID)

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
	tx := pg.DB.MustBegin()
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

	err := pg.DB.Get(&sum, `
		SELECT SUM(amount) as sum
		FROM transactions
		WHERE order_id = $1 AND status = 'completed'
	`, orderID)

	return sum, err
}

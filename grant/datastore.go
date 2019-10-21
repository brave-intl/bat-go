package grant

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/brave-intl/bat-go/datastore"
	"github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/garyburd/redigo/redis"
	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	// needed for magic migration
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Datastore abstracts over the underlying datastore
type Datastore interface {
	// UpsertWallet inserts the given wallet
	UpsertWallet(wallet *wallet.Info) error
	// GetClaimantID returns the providerID who has claimed a given grant
	GetClaimantProviderID(grant Grant) (string, error)
	// RedeemGrantForWallet redeems a claimed grant for a wallet
	RedeemGrantForWallet(grant Grant, wallet wallet.Info) error
	// ClaimGrantForWallet makes a claim to a particular Grant by a wallet
	ClaimGrantForWallet(grant Grant, wallet wallet.Info) error
	// HasGrantBeenRedeemed checks to see if a grant has been claimed
	HasGrantBeenRedeemed(grant Grant) (bool, error)
	// GetGrantsOrderedByExpiry returns ordered grant claims
	GetGrantsOrderedByExpiry(wallet wallet.Info) ([]Grant, error)
	// ClaimPromotionForWallet makes a claim to a particular promotion by a wallet
	ClaimPromotionForWallet(promo *promotion.Promotion, wallet *wallet.Info) (*promotion.Claim, error)
	// GetPromotion by ID
	GetPromotion(promotionID uuid.UUID) (*promotion.Promotion, error)
}

// Postgres is a WIP Datastore
type Postgres struct {
	*sqlx.DB
}

// NewMigrate creates a Migrate instance given a Postgres instance with an active database connection
func (pg *Postgres) NewMigrate() (*migrate.Migrate, error) {
	driver, err := postgres.WithInstance(pg.DB.DB, &postgres.Config{})
	if err != nil {
		return nil, err
	}

	dbMigrationsURL := os.Getenv("DATABASE_MIGRATIONS_URL")
	m, err := migrate.NewWithDatabaseInstance(
		dbMigrationsURL,
		"postgres",
		driver,
	)
	if err != nil {
		return nil, err
	}

	return m, err
}

// Migrate the Postgres instance
func (pg *Postgres) Migrate() error {
	m, err := pg.NewMigrate()
	if err != nil {
		return err
	}

	err = m.Migrate(1)
	if err != migrate.ErrNoChange && err != nil {
		return err
	}
	return nil
}

// NewPostgres creates a new Postgres Datastore
func NewPostgres(databaseURL string, performMigration bool) (*Postgres, error) {
	if len(databaseURL) == 0 {
		databaseURL = os.Getenv("DATABASE_URL")
	}

	db, err := sqlx.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}

	pg := &Postgres{db}

	if performMigration {
		err = pg.Migrate()
		if err != nil {
			return nil, err
		}
	}

	return pg, nil
}

// UpsertWallet upserts the given wallet
func (pg *Postgres) UpsertWallet(wallet *wallet.Info) error {
	statement := `
	insert into wallets (id, provider, provider_id, public_key)
	values ($1, $2, $3, $4)
	on conflict do nothing
	returning *`
	_, err := pg.DB.Exec(statement, wallet.ID, wallet.Provider, wallet.ProviderID, wallet.PublicKey)
	if err != nil {
		return err
	}

	return nil
}

// GetClaimantProviderID returns the providerID who has claimed a given grant
func (pg *Postgres) GetClaimantProviderID(grant Grant) (string, error) {
	wallet, err := pg.GetClaimant(grant)
	if wallet != nil {
		return wallet.ProviderID, err
	}
	return "", err
}

// GetClaimant returns info about the wallet which has claimed a given grant
func (pg *Postgres) GetClaimant(grant Grant) (*wallet.Info, error) {
	statement := `
	select
		wallets.*
	from wallets left join claims on wallets.id = claims.wallet_id where claims.id = $1;`
	wallets := []wallet.Info{}

	err := pg.DB.Select(&wallets, statement, grant.GrantID.String())
	if err != nil {
		return nil, err
	}

	if len(wallets) > 0 {
		return &wallets[0], nil
	}

	return nil, nil
}

// RedeemGrantForWallet redeems a claimed grant for a wallet
func (pg *Postgres) RedeemGrantForWallet(grant Grant, wallet wallet.Info) error {
	statement := `
	update claims
	set redeemed = true
	where id = $1 and promotion_id = $2 and wallet_id = $3 and not redeemed and legacy_claimed
	returning *`

	res, err := pg.DB.Exec(statement, grant.GrantID.String(), grant.PromotionID.String(), wallet.ID)
	if err != nil {
		return err
	}

	grantCount, err := res.RowsAffected()
	if err != nil {
		return err
	} else if grantCount < 1 {
		return errors.New("no matching claimed grant")
	} else if grantCount > 1 {
		return errors.New("more than one matching grant")
	}

	return nil
}

// ClaimGrantForWallet makes a claim to a particular GrantID by a wallet
func (pg *Postgres) ClaimGrantForWallet(grant Grant, wallet wallet.Info) error {
	statement := `
	insert into claims (id, promotion_id, wallet_id, approximate_value, legacy_claimed)
	values ($1, $2, $3, $4, true)
	returning *`

	// FIXME

	value := grant.AltCurrency.FromProbi(grant.Probi)

	_, err := pg.DB.Exec(statement, grant.GrantID.String(), grant.PromotionID.String(), wallet.ID, value)
	return err
}

// ClaimPromotionForWallet makes a claim to a particular promotion by a wallet
func (pg *Postgres) ClaimPromotionForWallet(promo *promotion.Promotion, wallet *wallet.Info) (*promotion.Claim, error) {
	tx, err := pg.DB.Beginx()
	if err != nil {
		return nil, err
	}

	// This will error if remaining_grants is insufficient due to constraint or the promotion is inactive
	res, err := tx.Exec(`update promotions set remaining_grants = remaining_grants - 1 where id = $1 and active`, promo.ID)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	promotionCount, err := res.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	} else if promotionCount != 1 {
		_ = tx.Rollback()
		return nil, errors.New("no matching active promotion")
	}

	claims := []promotion.Claim{}

	if promo.Type == "ads" {
		statement := `
		update claims
		set legacy_claimed = true
		where promotion_id = $1 and wallet_id = $2
		returning *`
		err = tx.Select(&claims, statement, promo.ID, wallet.ID)
	} else {
		statement := `
		insert into claims (promotion_id, wallet_id, approximate_value, legacy_claimed)
		values ($1, $2, $3, true)
		returning *`
		err = tx.Select(&claims, statement, promo.ID, wallet.ID, promo.ApproximateValue)
	}

	if err != nil {
		_ = tx.Rollback()
		return nil, err
	} else if len(claims) != 1 {
		_ = tx.Rollback()
		return nil, fmt.Errorf("Incorrect number of claims updated / inserted: %d", len(claims))
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return &claims[0], nil
}

// HasGrantBeenRedeemed checks to see if a grant has been claimed
func (pg *Postgres) HasGrantBeenRedeemed(grant Grant) (bool, error) {
	var redeemed []bool

	err := pg.DB.Select(&redeemed, "select redeemed from claims where id = $1", grant.GrantID.String())
	if err != nil {
		return false, err
	}

	if len(redeemed) != 1 {
		return false, errors.New("no matching claimed grant")
	}

	return redeemed[0], nil
}

// GetGrantsOrderedByExpiry returns ordered grant claims for a wallet
func (pg *Postgres) GetGrantsOrderedByExpiry(wallet wallet.Info) ([]Grant, error) {
	type GrantResult struct {
		Grant
		ApproximateValue decimal.Decimal `db:"approximate_value"`
		CreatedAt        time.Time       `json:"createdAt" db:"created_at"`
		ExpiresAt        time.Time       `json:"expiresAt" db:"expires_at"`
	}

	statement := `
select
	claims.id,
	promotions.approximate_value,
	claims.promotion_id,
	promotions.created_at,
	promotions.expires_at,
	promotions.promotion_type
from claims inner join promotions
on claims.promotion_id = promotions.id
where
	claims.wallet_id = $1 and
	not claims.redeemed and
	claims.legacy_claimed
	and promotions.expires_at > now()
order by promotions.expires_at`

	var grantResults []GrantResult

	err := pg.DB.Select(&grantResults, statement, wallet.ID)
	if err != nil {
		return []Grant{}, err
	}
	grants := make([]Grant, len(grantResults))

	for i, grant := range grantResults {
		{
			tmp := altcurrency.BAT
			grant.AltCurrency = &tmp
		}
		grant.Probi = grant.AltCurrency.ToProbi(grant.ApproximateValue)
		grant.MaturityTimestamp = grant.CreatedAt.Unix()
		grant.ExpiryTimestamp = grant.ExpiresAt.Unix()
		grants[i] = grant.Grant
	}

	return grants, nil
}

// GetPromotion by ID
func (pg *Postgres) GetPromotion(promotionID uuid.UUID) (*promotion.Promotion, error) {
	statement := "select * from promotions where id = $1"
	promotions := []promotion.Promotion{}
	err := pg.DB.Select(&promotions, statement, promotionID)
	if err != nil {
		return nil, err
	}

	if len(promotions) > 0 {
		return &promotions[0], nil
	}

	return nil, nil
}

// Redis is our legacy datastore
type Redis struct {
	*redis.Pool
}

// GetClaimantProviderID returns the providerID who has claimed a given grant
func (redis *Redis) GetClaimantProviderID(grant Grant) (string, error) {
	conn := redis.Pool.Get()
	defer closers.Panic(conn)
	kv := datastore.GetRedisKv(&conn)

	return getClaimantProviderID(&kv, grant)
}

func getClaimantProviderID(kv datastore.KvDatastore, grant Grant) (string, error) {
	return kv.Get(fmt.Sprintf(claimKeyFormat, grant.GrantID))
}

// RedeemGrantForWallet redeems a claimed grant for a wallet
func (redis *Redis) RedeemGrantForWallet(grant Grant, wallet wallet.Info) error {
	conn := redis.Pool.Get()
	defer closers.Panic(conn)
	redeemedGrants := datastore.GetRedisSet(&conn, "promotion:"+grant.PromotionID.String()+":grants")
	redeemedWallets := datastore.GetRedisSet(&conn, "promotion:"+grant.PromotionID.String()+":wallets")

	return redeemGrantForWallet(&redeemedGrants, &redeemedWallets, grant, wallet)
}

func redeemGrantForWallet(redeemedGrants datastore.SetLikeDatastore, redeemedWallets datastore.SetLikeDatastore, grant Grant, wallet wallet.Info) error {
	result, err := redeemedGrants.Add(grant.GrantID.String())
	if err != nil {
		return err
	}
	if !result {
		// a) this wallet has not yet redeemed a grant for the given promotionId
		return fmt.Errorf("grant %s has already been redeemed", grant.GrantID)
	}

	result, err = redeemedWallets.Add(wallet.ProviderID)
	if err != nil {
		return err
	}
	if !result {
		// b) this grant has not yet been redeemed by any wallet
		return fmt.Errorf("Wallet %s has already redeemed a grant from this promotion", wallet.ProviderID)
	}
	return nil
}

// ClaimGrantForWallet makes a claim to a particular GrantID by a wallet
func (redis *Redis) ClaimGrantForWallet(grant Grant, wallet wallet.Info) error {
	conn := redis.Pool.Get()
	defer closers.Panic(conn)
	kv := datastore.GetRedisKv(&conn)

	return claimGrantIDForWallet(&kv, grant.GrantID.String(), wallet)
}

func claimGrantIDForWallet(kv datastore.KvDatastore, grantID string, wallet wallet.Info) error {
	_, err := kv.Set(
		fmt.Sprintf(claimKeyFormat, grantID),
		wallet.ProviderID,
		ninetyDaysInSeconds,
		false,
	)
	if err != nil {
		return errors.New("An existing claim to the grant already exists")
	}
	return nil
}

// ClaimPromotionForWallet makes a claim to a particular promotion by a wallet
func (redis *Redis) ClaimPromotionForWallet(promotion *promotion.Promotion, wallet *wallet.Info) (*promotion.Claim, error) {
	return nil, nil
}

// HasGrantBeenRedeemed checks to see if a grant has been claimed
func (redis *Redis) HasGrantBeenRedeemed(grant Grant) (bool, error) {
	conn := redis.Pool.Get()
	defer closers.Panic(conn)
	redeemedGrants := datastore.GetRedisSet(&conn, "promotion:"+grant.PromotionID.String()+":grants")

	return hasGrantBeenRedeemed(&redeemedGrants, grant)
}

func hasGrantBeenRedeemed(redeemedGrants datastore.SetLikeDatastore, grant Grant) (bool, error) {
	return redeemedGrants.Contains(grant.GrantID.String())
}

// GetGrantsOrderedByExpiry returns ordered grant claims for a wallet
func (redis *Redis) GetGrantsOrderedByExpiry(wallet wallet.Info) ([]Grant, error) {
	return []Grant{}, nil
}

// GetPromotion by ID
func (redis *Redis) GetPromotion(promotionID uuid.UUID) (*promotion.Promotion, error) {
	return nil, nil
}

// InMemory is an unsafe Datastore that keeps data in memory, used for testing
type InMemory struct {
}

// GetClaimantProviderID returns the providerID who has claimed a given grant
func (inmem *InMemory) GetClaimantProviderID(grant Grant) (string, error) {
	kv, err := datastore.GetKvDatastore(context.Background())
	if err != nil {
		return "", err
	}

	return getClaimantProviderID(kv, grant)
}

// RedeemGrantForWallet redeems a claimed grant for a wallet
func (inmem *InMemory) RedeemGrantForWallet(grant Grant, wallet wallet.Info) error {
	ctx := context.Background()
	redeemedGrants, err := datastore.GetSetDatastore(ctx, "promotion:"+grant.PromotionID.String()+":grants")
	if err != nil {
		return err
	}
	redeemedWallets, err := datastore.GetSetDatastore(ctx, "promotion:"+grant.PromotionID.String()+":wallets")
	if err != nil {
		return err
	}

	return redeemGrantForWallet(redeemedGrants, redeemedWallets, grant, wallet)
}

// ClaimGrantForWallet makes a claim to a particular Grant by a wallet
func (inmem *InMemory) ClaimGrantForWallet(grant Grant, wallet wallet.Info) error {
	kv, err := datastore.GetKvDatastore(context.Background())
	if err != nil {
		return err
	}

	return claimGrantIDForWallet(kv, grant.GrantID.String(), wallet)
}

// ClaimPromotionForWallet makes a claim to a particular promotion by a wallet
func (inmem *InMemory) ClaimPromotionForWallet(promotion *promotion.Promotion, wallet *wallet.Info) (*promotion.Claim, error) {
	return nil, nil
}

// HasGrantBeenRedeemed checks to see if a grant has been claimed
func (inmem *InMemory) HasGrantBeenRedeemed(grant Grant) (bool, error) {
	redeemedGrants, err := datastore.GetSetDatastore(context.Background(), "promotion:"+grant.PromotionID.String()+":grants")
	if err != nil {
		return false, err
	}

	return hasGrantBeenRedeemed(redeemedGrants, grant)
}

// GetGrantsOrderedByExpiry returns ordered grant claims for a wallet
func (inmem *InMemory) GetGrantsOrderedByExpiry(wallet wallet.Info) ([]Grant, error) {
	return []Grant{}, nil
}

// UpsertWallet upserts the given wallet
func (inmem *InMemory) UpsertWallet(wallet *wallet.Info) error {
	return nil
}

// GetPromotion by ID
func (inmem *InMemory) GetPromotion(promotionID uuid.UUID) (*promotion.Promotion, error) {
	return nil, nil
}

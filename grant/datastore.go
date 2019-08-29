package grant

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/brave-intl/bat-go/datastore"
	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/garyburd/redigo/redis"
	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/jmoiron/sqlx"
	// needed for magic migration
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Datastore abstracts over the underlying datastore
type Datastore interface {
	// GetClaimantID returns the providerID who has claimed a given grant
	GetClaimantProviderID(grant Grant) (string, error)
	// RedeemGrantForWallet redeems a claimed grant for a wallet
	RedeemGrantForWallet(grant Grant, wallet wallet.Info) error
	// ClaimGrantForWallet makes a claim to a particular Grant by a wallet
	ClaimGrantForWallet(grant Grant, wallet wallet.Info) error
	// HasGrantBeenRedeemed checks to see if a grant has been claimed
	HasGrantBeenRedeemed(grant Grant) (bool, error)
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
	from wallets left join claims on wallets.id = claims.wallet_id and claims.id = $1;`
	wallets := []wallet.Info{}

	err := pg.DB.Select(&wallets, statement, grant.GrantID)
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
	// FIXME should this limit version?
	statement := `
	update claims
	set redeemed = true
	where grant_id = $1 and promotion_id = $2 and wallet_id = $2
	returning *`

	res, err := pg.DB.Exec(statement, grant.GrantID, grant.PromotionID, wallet.ID)
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

// ClaimGrantIDForWallet makes a claim to a particular GrantID by a wallet
func (pg *Postgres) ClaimGrantIDForWallet(grant Grant, wallet wallet.Info) error {
	statement := `
	insert into claims (id, promotion_id, wallet_id, approximate_value)
	values ($1, $2, $3, $4)
	returning *`

	value := grant.AltCurrency.FromProbi(grant.Probi)

	_, err := pg.DB.Exec(statement, grant.GrantID, grant.PromotionID, wallet.ID, value)
	return err
}

// HasGrantBeenRedeemed checks to see if a grant has been claimed
func (pg *Postgres) HasGrantBeenRedeemed(grant Grant) (bool, error) {
	var redeemed []bool

	err := pg.DB.Select(&redeemed, "select redeemed from claims where grant_id = $1", grant.GrantID)
	if err != nil {
		return false, err
	}

	if len(redeemed) != 1 {
		return false, errors.New("no matching claimed grant")
	}

	return redeemed[0], nil
}

// TODO implement postgres datastore
// Can set up 1:1 correspondance between claim ID and grant ID?
// Ensure only version < 4 promotions can go through legacy redeem
//
// Cutover:

// 1. Disable claim through old API
// 2. Take db dump of wallets collection
// 3. Recreate promotions / claims table from wallets collection
// 4. Take ledger server down
// 5. Sync redemption status from redis
// 6. Upgrade grant server?

// Redis is our current datastore
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

// HasGrantBeenRedeemed checks to see if a grant has been claimed
func (inmem *InMemory) HasGrantBeenRedeemed(grant Grant) (bool, error) {
	redeemedGrants, err := datastore.GetSetDatastore(context.Background(), "promotion:"+grant.PromotionID.String()+":grants")
	if err != nil {
		return false, err
	}

	return hasGrantBeenRedeemed(redeemedGrants, grant)
}

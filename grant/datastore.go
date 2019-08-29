package grant

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/brave-intl/bat-go/datastore"
	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/garyburd/redigo/redis"
	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
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
	*sql.DB
}

// NewPostgres creates a new Postgres Datastore
func NewPostgres(databaseURL string) (*Postgres, error) {
	if len(databaseURL) == 0 {
		databaseURL = os.Getenv("DATABASE_URL")
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return nil, err
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file:///src/migrations",
		"postgres", driver)
	if err != nil {
		return nil, err
	}
	err = m.Migrate(1)
	if err != migrate.ErrNoChange && err != nil {
		return nil, err
	}
	return &Postgres{db}, nil
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

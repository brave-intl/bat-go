package grant

import (
	"errors"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/wallet"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"

	// needed for magic migration
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Datastore abstracts over the underlying datastore
type Datastore interface {
	// UpsertWallet inserts the given wallet
	UpsertWallet(wallet *wallet.Info) error
	// RedeemGrantForWallet redeems a claimed grant for a wallet
	RedeemGrantForWallet(grant Grant, wallet wallet.Info) error
	// GetGrantsOrderedByExpiry returns ordered grant claims with optional promotion type filter
	GetGrantsOrderedByExpiry(wallet wallet.Info, promotionType string) ([]Grant, error)
	// ClaimPromotionForWallet makes a claim to a particular promotion by a wallet
	ClaimPromotionForWallet(promo *promotion.Promotion, wallet *wallet.Info) (*promotion.Claim, error)
	// GetPromotion by ID
	GetPromotion(promotionID uuid.UUID) (*promotion.Promotion, error)
}

// ReadOnlyDatastore includes all database methods that can be made with a read only db connection
type ReadOnlyDatastore interface {
	// GetGrantsOrderedByExpiry returns ordered grant claims with optional promotion type filter
	GetGrantsOrderedByExpiry(wallet wallet.Info, promotionType string) ([]Grant, error)
	// GetPromotion by ID
	GetPromotion(promotionID uuid.UUID) (*promotion.Promotion, error)
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

// RedeemGrantForWallet redeems a claimed grant for a wallet
func (pg *Postgres) RedeemGrantForWallet(grant Grant, wallet wallet.Info) error {
	statement := `
	update claims
	set redeemed = true, redeemed_at = current_timestamp
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

// ClaimPromotionForWallet makes a claim to a particular promotion by a wallet
func (pg *Postgres) ClaimPromotionForWallet(promo *promotion.Promotion, wallet *wallet.Info) (*promotion.Claim, error) {
	tx, err := pg.DB.Beginx()
	if err != nil {
		return nil, err
	}
	defer pg.RollbackTx(tx)

	// This will error if remaining_grants is insufficient due to constraint or the promotion is inactive
	res, err := tx.Exec(`update promotions set remaining_grants = remaining_grants - 1 where id = $1 and active`, promo.ID)
	if err != nil {
		return nil, err
	}
	promotionCount, err := res.RowsAffected()
	if err != nil {
		return nil, err
	} else if promotionCount != 1 {
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
		return nil, err
	} else if len(claims) != 1 {
		return nil, fmt.Errorf("Incorrect number of claims updated / inserted: %d", len(claims))
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return &claims[0], nil
}

// GetGrantsOrderedByExpiry returns ordered grant claims for a wallet
func (pg *Postgres) GetGrantsOrderedByExpiry(wallet wallet.Info, promotionType string) ([]Grant, error) {
	type GrantResult struct {
		Grant
		ApproximateValue decimal.Decimal `db:"approximate_value"`
		CreatedAt        time.Time       `db:"created_at"`
		ExpiresAt        time.Time       `db:"expires_at"`
		Platform         string          `db:"platform"`
	}

	if len(promotionType) == 0 {
		promotionType = "{ads,ugp}"
	}

	statement := `
select
	claims.id,
	claims.approximate_value,
	claims.promotion_id,
	promotions.created_at,
	promotions.expires_at,
	promotions.promotion_type,
	promotions.platform
from claims inner join promotions
on claims.promotion_id = promotions.id
where
	claims.wallet_id = $1 and
	not claims.redeemed and
	claims.legacy_claimed and
	promotions.promotion_type = any($2::text[]) and
	promotions.expires_at > now()
order by promotions.expires_at`

	var grantResults []GrantResult

	err := pg.DB.Select(&grantResults, statement, wallet.ID, promotionType)
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
		if grant.Type == "ugp" && grant.Platform == "android" {
			grant.Type = "android"
		}
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

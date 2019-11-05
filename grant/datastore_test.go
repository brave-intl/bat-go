// +build integration

package grant

import (
	"sort"
	"testing"

	"github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/wallet"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type PostgresTestSuite struct {
	suite.Suite
}

func (suite *PostgresTestSuite) SetupSuite() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	m, err := pg.NewMigrate()
	suite.Require().NoError(err, "Failed to create migrate instance")

	ver, dirty, _ := m.Version()
	if dirty {
		suite.Require().NoError(m.Force(int(ver)))
	}
	if ver > 0 {
		suite.Require().NoError(m.Down(), "Failed to migrate down cleanly")
	}

	suite.Require().NoError(pg.Migrate(), "Failed to fully migrate")
}

func (suite *PostgresTestSuite) SetupTest() {
	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions"}

	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.DB.Exec("delete from " + table)
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func (suite *PostgresTestSuite) TestGetGrantsOrderedByExpiry() {
	w := wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: ""}
	var grant1 Grant
	var grant2 Grant
	{
		pg, err := promotion.NewPostgres("", false)
		suite.Require().NoError(err)

		suite.Require().NoError(pg.InsertWallet(&w), "Save wallet should succeed")

		promotion1, err := pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(25.0), "")
		suite.Require().NoError(err, "Create promotion should succeed")
		suite.Require().NoError(pg.ActivatePromotion(promotion1), "Activate promotion should succeed")

		_, err = pg.DB.Exec("update promotions set created_at = now() - interval '1 month', expires_at = now() + interval '3 months' where id = $1", promotion1.ID)
		suite.Require().NoError(err, "Changing promotion created_at / expires_at must succeed")

		promotion2, err := pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(15.0), "android")
		suite.Require().NoError(err, "Create promotion should succeed")
		suite.Require().NoError(pg.ActivatePromotion(promotion2), "Activate promotion should succeed")

		{
			tmp := altcurrency.BAT
			grant1.AltCurrency = &tmp
			grant2.AltCurrency = &tmp
		}
		grant1.PromotionID = promotion1.ID
		grant2.PromotionID = promotion2.ID
		grant1.GrantID = uuid.NewV4()
		grant2.GrantID = uuid.NewV4()
		grant1.Probi = grant1.AltCurrency.ToProbi(promotion1.ApproximateValue)
		grant2.Probi = grant2.AltCurrency.ToProbi(promotion2.ApproximateValue)
	}

	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	err = pg.ClaimGrantForWallet(grant1, w)
	suite.Assert().NoError(err, "Claim for wallet should succeed, promotion is active and has grants left")

	err = pg.ClaimGrantForWallet(grant1, w)
	suite.Assert().Error(err, "Re-claim for wallet should fail")

	err = pg.ClaimGrantForWallet(grant2, w)
	suite.Assert().NoError(err, "Claim for wallet should succeed, promotion is active and has grants left")

	grants, err := pg.GetGrantsOrderedByExpiry(w, "")
	suite.Assert().NoError(err, "Get grants ordered by expiry should succeed")

	grantsSorted := make([]Grant, len(grants))
	copy(grantsSorted, grants)
	sort.Sort(ByExpiryTimestamp(grantsSorted))
	suite.Assert().Equal(grants, grantsSorted)

	suite.Assert().Equal(grant1.GrantID, grants[0].GrantID)
	// Check legacy grant type compatibility translation
	suite.Assert().Equal("android", grants[1].Type)
}

func TestPostgresTestSuite(t *testing.T) {
	suite.Run(t, new(PostgresTestSuite))
}

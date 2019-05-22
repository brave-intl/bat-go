// +build integration

package promotion

import (
	"testing"

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

func (suite *PostgresTestSuite) TestCreatePromotion() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	value := decimal.NewFromFloat(25.0)
	numGrants := 10

	promotion, err := pg.CreatePromotion("ugp", numGrants, value)
	suite.Assert().NoError(err, "Create promotion should succeed")

	suite.Assert().Equal(numGrants, promotion.RemainingGrants)
	suite.Assert().True(value.Equal(promotion.ApproximateValue))
}

func (suite *PostgresTestSuite) TestGetPromotion() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	value := decimal.NewFromFloat(25.0)
	numGrants := 10

	promotion, err := pg.CreatePromotion("ugp", numGrants, value)
	suite.Assert().NoError(err, "Create promotion should succeed")

	promotion, err = pg.GetPromotion(promotion.ID)
	suite.Assert().NoError(err, "Get promotion should succeed")

	suite.Assert().Equal(numGrants, promotion.RemainingGrants)
	suite.Assert().True(value.Equal(promotion.ApproximateValue))
}

func (suite *PostgresTestSuite) TestActivatePromotion() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	promotion, err := pg.CreatePromotion("ugp", 1, decimal.NewFromFloat(25.0))
	suite.Assert().NoError(err, "Create promotion should succeed")

	suite.Assert().False(promotion.Active)

	suite.Assert().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotion, err = pg.GetPromotion(promotion.ID)
	suite.Assert().NoError(err, "Get promotion should succeed")

	suite.Assert().True(promotion.Active)
}

func (suite *PostgresTestSuite) TestInsertIssuer() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	promotion, err := pg.CreatePromotion("ugp", 10, decimal.NewFromFloat(25.0))
	suite.Assert().NoError(err, "Create promotion should succeed")

	issuer := Issuer{PromotionID: promotion.ID, Cohort: "test", PublicKey: publicKey}
	suite.Assert().NoError(pg.InsertIssuer(&issuer), "Save issuer should succeed")
}

func (suite *PostgresTestSuite) TestGetIssuer() {
	// FIXME
}

func (suite *PostgresTestSuite) TestInsertWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	wallet := &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Assert().NoError(pg.InsertWallet(wallet), "Save wallet should succeed")
}

func (suite *PostgresTestSuite) TestGetWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	id := uuid.NewV4()

	origWallet := &wallet.Info{ID: id.String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Assert().NoError(pg.InsertWallet(origWallet), "Save wallet should succeed")

	wallet, err := pg.GetWallet(id)
	suite.Assert().NoError(err, "Get wallet should succeed")
	suite.Assert().Equal(origWallet, wallet)
}

func (suite *PostgresTestSuite) TestCreateClaim() {
	// TODO
}

func (suite *PostgresTestSuite) TestClaimForWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	blindedCreds := JSONStringArray([]string{})

	promotion, err := pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(25.0))
	suite.Require().NoError(err, "Create promotion should succeed")

	w := &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")

	_, err = pg.ClaimForWallet(promotion, w, blindedCreds)
	suite.Assert().Error(err, "Claim for wallet should fail, promotion is not active")

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	_, err = pg.ClaimForWallet(promotion, w, blindedCreds)
	suite.Assert().NoError(err, "Claim for wallet should succeed, promotion is active and has grants left")
	_, err = pg.ClaimForWallet(promotion, w, blindedCreds)
	suite.Assert().Error(err, "Claim for wallet should fail, wallet already claimed this promotion")

	w = &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")
	_, err = pg.ClaimForWallet(promotion, w, blindedCreds)
	suite.Assert().NoError(err, "Claim for wallet should succeed, promotion is active and has grants left")

	w = &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")
	_, err = pg.ClaimForWallet(promotion, w, blindedCreds)
	suite.Assert().Error(err, "Claim for wallet should fail, promotion is active but has no more grants")

	promotion, err = pg.CreatePromotion("ads", 2, decimal.NewFromFloat(25.0))
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	w = &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")

	_, err = pg.CreateClaim(promotion.ID, w.ID, decimal.NewFromFloat(30.0))
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")

	w2 := &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w2), "Save wallet should succeed")
	_, err = pg.ClaimForWallet(promotion, w2, blindedCreds)
	suite.Assert().Error(err, "Claim for wallet should fail, wallet does not have pre-registered claim")

	_, err = pg.ClaimForWallet(promotion, w, blindedCreds)
	suite.Assert().NoError(err, "Claim for wallet should succeed, wallet has pre-registered claim")

	promotion, err = pg.GetPromotion(promotion.ID)
	suite.Assert().NoError(err, "Get promotion should succeed")
	suite.Assert().Equal(1, promotion.RemainingGrants)
}

func (suite *PostgresTestSuite) TestGetAvailablePromotionsForWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	//blindedCreds := JSONStringArray([]string{})

	w := &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")

	promotions, err := pg.GetAvailablePromotionsForWallet(w)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	promotion, err := pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(25.0))
	suite.Require().NoError(err, "Create promotion should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().Equal(*promotion, promotions[0])
	suite.Assert().False(promotions[0].Available)

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().True(promotions[0].Active)
	suite.Assert().True(promotions[0].Available)

	promotion, err = pg.CreatePromotion("ads", 2, decimal.NewFromFloat(25.0))
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(2, len(promotions))
	suite.Assert().True(promotions[0].Available)
	suite.Assert().False(promotions[1].Available)

	_, err = pg.CreateClaim(promotion.ID, w.ID, decimal.NewFromFloat(30.0))
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(2, len(promotions))
	suite.Assert().True(promotions[0].Available)
	suite.Assert().True(promotions[1].Available)
}

func (suite *PostgresTestSuite) TestGetClaimCreds() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	blindedCreds := JSONStringArray([]string{"hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="})

	promotion, err := pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(25.0))
	suite.Require().NoError(err, "Create promotion should succeed")

	w := &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	claim, err := pg.ClaimForWallet(promotion, w, blindedCreds)
	suite.Assert().NoError(err, "Claim for wallet should succeed, promotion is active and has grants left")

	claimCreds, err := pg.GetClaimCreds(claim.ID)
	suite.Assert().NoError(err, "Get claim creds should succeed")

	suite.Assert().Equal(blindedCreds, claimCreds.BlindedCreds)
}

func (suite *PostgresTestSuite) TestSaveClaimCreds() {
	// FIXME
}

func TestPostgresTestSuite(t *testing.T) {
	suite.Run(t, new(PostgresTestSuite))
}

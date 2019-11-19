// +build integration

package promotion

import (
	"context"
	"testing"

	"github.com/brave-intl/bat-go/wallet"
	gomock "github.com/golang/mock/gomock"
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
	suite.CleanDB()
}

func (suite *PostgresTestSuite) CleanDB() {
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

	promotion, err := pg.CreatePromotion("ugp", numGrants, value, "")
	suite.Assert().NoError(err, "Create promotion should succeed")

	suite.Assert().Equal(numGrants, promotion.RemainingGrants)
	suite.Assert().True(value.Equal(promotion.ApproximateValue))
}

func (suite *PostgresTestSuite) TestGetPromotion() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	value := decimal.NewFromFloat(25.0)
	numGrants := 10

	promotion, err := pg.CreatePromotion("ugp", numGrants, value, "")
	suite.Assert().NoError(err, "Create promotion should succeed")

	promotion, err = pg.GetPromotion(promotion.ID)
	suite.Assert().NoError(err, "Get promotion should succeed")

	suite.Assert().Equal(numGrants, promotion.RemainingGrants)
	suite.Assert().True(value.Equal(promotion.ApproximateValue))
}

func (suite *PostgresTestSuite) TestActivatePromotion() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	promotion, err := pg.CreatePromotion("ugp", 1, decimal.NewFromFloat(25.0), "")
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

	promotion, err := pg.CreatePromotion("ugp", 10, decimal.NewFromFloat(25.0), "")
	suite.Assert().NoError(err, "Create promotion should succeed")

	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "test", PublicKey: publicKey}
	_, err = pg.InsertIssuer(issuer)

	suite.Assert().NoError(err, "Save issuer should succeed")
}

func (suite *PostgresTestSuite) TestGetIssuer() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	promotion, err := pg.CreatePromotion("ugp", 10, decimal.NewFromFloat(25.0), "")
	suite.Assert().NoError(err, "Create promotion should succeed")

	origIssuer := &Issuer{PromotionID: promotion.ID, Cohort: "test", PublicKey: publicKey}
	origIssuer, err = pg.InsertIssuer(origIssuer)
	suite.Assert().NoError(err, "Insert issuer should succeed")

	issuerByPromoAndCohort, err := pg.GetIssuer(promotion.ID, "test")
	suite.Assert().NoError(err, "Get issuer should succeed")
	suite.Assert().Equal(origIssuer, issuerByPromoAndCohort)

	issuerByPublicKey, err := pg.GetIssuerByPublicKey(publicKey)
	suite.Assert().NoError(err, "Get issuer by public key should succeed")
	suite.Assert().Equal(origIssuer, issuerByPublicKey)
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
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	promotion, err := pg.CreatePromotion("ads", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	w := &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")

	_, err = pg.CreateClaim(promotion.ID, w.ID, decimal.NewFromFloat(30.0), decimal.NewFromFloat(0))
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")
}

func (suite *PostgresTestSuite) TestGetPreClaim() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	promotion, err := pg.CreatePromotion("ads", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	w := &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")

	expectedClaim, err := pg.CreateClaim(promotion.ID, w.ID, decimal.NewFromFloat(30.0), decimal.NewFromFloat(0))
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")

	claim, err := pg.GetPreClaim(promotion.ID, w.ID)
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().Equal(expectedClaim, claim)
}

func (suite *PostgresTestSuite) TestClaimForWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	blindedCreds := JSONStringArray([]string{})

	promotion, err := pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")

	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Assert().NoError(err, "Insert issuer should succeed")

	w := &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")

	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Assert().Error(err, "Claim for wallet should fail, promotion is not active")

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Assert().NoError(err, "Claim for wallet should succeed, promotion is active and has grants left")
	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Assert().Error(err, "Claim for wallet should fail, wallet already claimed this promotion")

	w = &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")
	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Assert().NoError(err, "Claim for wallet should succeed, promotion is active and has grants left")

	w = &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")
	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Assert().Error(err, "Claim for wallet should fail, promotion is active but has no more grants")

	promotion, err = pg.CreatePromotion("ads", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	w = &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")

	_, err = pg.CreateClaim(promotion.ID, w.ID, decimal.NewFromFloat(30.0), decimal.NewFromFloat(0))
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")

	w2 := &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w2), "Save wallet should succeed")
	_, err = pg.ClaimForWallet(promotion, issuer, w2, blindedCreds)
	suite.Assert().Error(err, "Claim for wallet should fail, wallet does not have pre-registered claim")

	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Assert().NoError(err, "Claim for wallet should succeed, wallet has pre-registered claim")

	promotion, err = pg.GetPromotion(promotion.ID)
	suite.Assert().NoError(err, "Get promotion should succeed")
	suite.Assert().Equal(1, promotion.RemainingGrants)
}

func (suite *PostgresTestSuite) TestGetAvailablePromotionsForWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	w := &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")

	promotions, err := pg.GetAvailablePromotionsForWallet(w, "", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	promotion, err := pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	promotion.PublicKeys = JSONStringArray{}

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().True(promotions[0].Active)
	suite.Assert().True(promotions[0].Available)

	promotion, err = pg.CreatePromotion("ads", 2, decimal.NewFromFloat(35.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().True(promotions[0].Available)

	// 30.7 * 4 = 122.8 => test differences in rounding
	adClaimValue := decimal.NewFromFloat(30.7)
	claim, err := pg.CreateClaim(promotion.ID, w.ID, adClaimValue, decimal.NewFromFloat(0))
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")
	adSuggestionsPerGrant, err := claim.SuggestionsNeeded(promotion)
	suite.Require().NoError(err)

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(2, len(promotions))
	suite.Assert().True(promotions[0].Available)
	suite.Assert().True(promotions[1].Available)
	suite.Assert().True(adClaimValue.Equals(promotions[1].ApproximateValue))
	suite.Assert().Equal(adSuggestionsPerGrant, promotions[1].SuggestionsPerGrant)
}

func (suite *PostgresTestSuite) TestGetAvailablePromotions() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	promotions, err := pg.GetAvailablePromotions("", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	promotion, err := pg.CreatePromotion("ugp", 0, decimal.NewFromFloat(15.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	promotion.PublicKeys = JSONStringArray{}
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotions("", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions), "Active promo with no grants should not appears in legacy list")

	suite.CleanDB()

	promotion, err = pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	promotion.PublicKeys = JSONStringArray{}

	promotions, err = pg.GetAvailablePromotions("", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	promotions, err = pg.GetAvailablePromotions("", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotions("", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().True(promotions[0].Active)
	suite.Assert().True(promotions[0].Available)

	promotions, err = pg.GetAvailablePromotions("", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().True(promotions[0].Active)
	suite.Assert().True(promotions[0].Available)

	promotion, err = pg.CreatePromotion("ads", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotions("", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().True(promotions[0].Active)
	suite.Assert().True(promotions[0].Available)

	promotions, err = pg.GetAvailablePromotions("", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().True(promotions[0].Active)
	suite.Assert().True(promotions[0].Available)

	// Test platform='desktop' returns all desktop grants for non-legacy
	// GetAvailablePromotions endpoint w/o paymentID
	suite.CleanDB()

	// Create desktop promotion
	promotion, err = pg.CreatePromotion("ugp", 1, decimal.NewFromFloat(25.0), "desktop")
	suite.Require().NoError(err, "Create promotion should succeed")
	err = pg.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Activate promotion should succeed")

	// Ensure they are all returned
	promotions, err = pg.GetAvailablePromotions("desktop", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	promotions, err = pg.GetAvailablePromotions("osx", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	promotions, err = pg.GetAvailablePromotions("linux", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	promotions, err = pg.GetAvailablePromotions("windows", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	// Test platform='desktop' returns all desktop grants for legacy
	// GetAvailablePromotions endpoint without paymentID
	suite.CleanDB()

	promotion, err = pg.CreatePromotion("ugp", 1, decimal.NewFromFloat(25.0), "desktop")
	suite.Require().NoError(err, "Create promotion should succeed")
	err = pg.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Activate promotion should succeed")

	// Ensure they are all returned
	// Legacy endpoints only return active
	err = pg.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotions("desktop", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	promotions, err = pg.GetAvailablePromotions("osx", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	promotions, err = pg.GetAvailablePromotions("linux", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	promotions, err = pg.GetAvailablePromotions("windows", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	suite.CleanDB()

	// Create desktop promotion
	promotion, err = pg.CreatePromotion("ugp", 1, decimal.NewFromFloat(25.0), "ios")
	suite.Require().NoError(err, "Create promotion should succeed")

	// it should not be in the legacy list until activated
	promotions, err = pg.GetAvailablePromotions("ios", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	err = pg.ActivatePromotion(promotion)

	promotions, err = pg.GetAvailablePromotions("ios", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))

	// Desktop should not see an iOS grant
	promotions, err = pg.GetAvailablePromotions("desktop", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	// But iOS should
	promotions, err = pg.GetAvailablePromotions("ios", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
}

func (suite *PostgresTestSuite) TestGetAvailablePromotionsForWalletLegacy() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	w := &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")
	w2 := &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w2), "Save wallet should succeed")

	promotions, err := pg.GetAvailablePromotionsForWallet(w, "", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	promotion, err := pg.CreatePromotion("ugp", 1, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions), "Legacy listing should not show inactive promotions")

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().True(promotions[0].Active)
	suite.Assert().True(promotions[0].Available)

	// Simulate legacy claim
	claim, err := pg.CreateClaim(promotion.ID, w.ID, decimal.NewFromFloat(25.0), decimal.NewFromFloat(0))
	suite.Require().NoError(err, "Creating claim should succeed")
	_, err = pg.DB.Exec("update claims set legacy_claimed = true where claims.id = $1", claim.ID)
	suite.Require().NoError(err, "Setting legacy_claimed should succeed")
	_, err = pg.DB.Exec(`update promotions set remaining_grants = remaining_grants - 1 where id = $1 and active`, promotion.ID)
	suite.Require().NoError(err, "Setting remaining grants should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions), "Legacy claimed promotions should not appear in legacy list")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions), "Legacy claimed promotions should appear in non-legacy list")
	suite.Assert().True(promotions[0].Active)
	suite.Assert().True(promotions[0].Available)

	promotions, err = pg.GetAvailablePromotionsForWallet(w2, "", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions), "Promotion with one grant should not appear after one claim")

	promotion, err = pg.CreatePromotion("ads", 1, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions), "Unavailable ads promo should not appear")

	// Create pre-registered ads claim
	claim, err = pg.CreateClaim(promotion.ID, w.ID, decimal.NewFromFloat(30.0), decimal.NewFromFloat(0))
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().True(promotions[0].Available)

	// Simulate legacy claim
	_, err = pg.DB.Exec("update claims set legacy_claimed = true where claims.id = $1", claim.ID)
	suite.Require().NoError(err, "Setting legacy_claimed should succeed")
	_, err = pg.DB.Exec(`update promotions set remaining_grants = remaining_grants - 1 where id = $1 and active`, promotion.ID)
	suite.Require().NoError(err, "Setting remaining grants should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "", true)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions), "Legacy claimed promotions should not appear in legacy list")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "", false)
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(2, len(promotions), "Legacy claimed promotions should appear in non-legacy list")
	suite.Assert().True(promotions[0].Available)
	suite.Assert().True(promotions[1].Available)
}

func (suite *PostgresTestSuite) TestGetClaimCreds() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	blindedCreds := JSONStringArray([]string{"hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="})

	promotion, err := pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")

	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Assert().NoError(err, "Insert issuer should succeed")

	w := &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	claim, err := pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Assert().NoError(err, "Claim for wallet should succeed, promotion is active and has grants left")

	claimCreds, err := pg.GetClaimCreds(claim.ID)
	suite.Assert().NoError(err, "Get claim creds should succeed")

	suite.Assert().Equal(blindedCreds, claimCreds.BlindedCreds)
}

func (suite *PostgresTestSuite) TestGetClaimByWalletAndPromotion() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	blindedCreds := JSONStringArray([]string{"hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="})
	w := &wallet.Info{
		ID:         uuid.NewV4().String(),
		Provider:   "uphold",
		ProviderID: uuid.NewV4().String(),
		PublicKey:  publicKey,
	}
	err = pg.InsertWallet(w)

	// Create promotion
	promotion, err := pg.CreatePromotion(
		"ugp",
		2,
		decimal.NewFromFloat(50.0),
		"",
	)
	suite.Assert().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Assert().NoError(err, "Insert issuer should succeed")

	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Assert().NoError(err, "Claim creation should succeed")

	// First try to look up a a claim for a wallet that doesn't have one
	fakeWallet := &wallet.Info{ID: uuid.NewV4().String()}
	claim, err := pg.GetClaimByWalletAndPromotion(fakeWallet, promotion)
	suite.Assert().NoError(err, "Get claim by wallet and promotion should succeed")
	suite.Assert().Nil(claim)

	// Now look up claim for wallet that does have one
	claim, err = pg.GetClaimByWalletAndPromotion(w, promotion)
	suite.Assert().NoError(err, "Get claim by wallet and promotion should succeed")
	suite.Assert().Equal(claim.PromotionID, promotion.ID)
	suite.Assert().Equal(claim.WalletID.String(), w.ID)

	promotion, err = pg.CreatePromotion("ads", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	_, err = pg.CreateClaim(promotion.ID, w.ID, decimal.NewFromFloat(30.0), decimal.NewFromFloat(0))
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")

	// A preregistered claim should not exist
	claim, err = pg.GetClaimByWalletAndPromotion(w, promotion)
	suite.Assert().NoError(err, "Get claim by wallet and promotion should succeed")
	suite.Assert().Nil(claim)
}

func (suite *PostgresTestSuite) TestSaveClaimCreds() {
	// FIXME
}

func (suite *PostgresTestSuite) TestRunNextClaimJob() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	mockClaimWorker := NewMockClaimWorker(mockCtrl)

	attempted, err := pg.RunNextClaimJob(context.Background(), mockClaimWorker)
	suite.Require().Equal(false, attempted)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	blindedCreds := JSONStringArray([]string{"hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="})
	signedCreds := JSONStringArray([]string{"hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="})
	batchProof := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	promotion, err := pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")

	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Assert().NoError(err, "Insert issuer should succeed")

	w := &wallet.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(w), "Save wallet should succeed")

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	claim, err := pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Assert().NoError(err, "Claim for wallet should succeed, promotion is active and has grants left")

	creds := &ClaimCreds{
		ID:           claim.ID,
		BlindedCreds: blindedCreds,
		SignedCreds:  &signedCreds,
		BatchProof:   &batchProof,
		PublicKey:    &issuer.PublicKey,
	}

	// One signing job should run
	mockClaimWorker.EXPECT().SignClaimCreds(gomock.Any(), gomock.Eq(claim.ID), gomock.Eq(*issuer), gomock.Eq([]string(blindedCreds))).Return(creds, nil)
	attempted, err = pg.RunNextClaimJob(context.Background(), mockClaimWorker)
	suite.Require().Equal(true, attempted)
	suite.Require().NoError(err)

	// No further jobs should run
	attempted, err = pg.RunNextClaimJob(context.Background(), mockClaimWorker)
	suite.Require().Equal(false, attempted)
	suite.Require().NoError(err)
}

func TestPostgresTestSuite(t *testing.T) {
	suite.Run(t, new(PostgresTestSuite))
}

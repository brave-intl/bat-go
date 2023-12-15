//go:build integration

package promotion

import (
	"context"
	"errors"

	"github.com/brave-intl/bat-go/libs/jsonutils"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/services/wallet"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type PostgresTestSuite struct {
	suite.Suite
}

func (suite *PostgresTestSuite) SetupSuite() {
	pg, _, err := NewPostgres()
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

func (suite *PostgresTestSuite) TearDownTest() {
	suite.CleanDB()
}

func (suite *PostgresTestSuite) CleanDB() {
	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions", "claim_drain"}

	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.RawDB().Exec("delete from " + table)
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func (suite *PostgresTestSuite) TestCreatePromotion() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	value := decimal.NewFromFloat(25.0)
	numGrants := 10

	promotion, err := pg.CreatePromotion("ugp", numGrants, value, "")
	suite.Require().NoError(err, "Create promotion should succeed")

	suite.Require().Equal(numGrants, promotion.RemainingGrants)
	suite.Require().True(value.Equal(promotion.ApproximateValue))
}

func (suite *PostgresTestSuite) TestGetPromotion() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	value := decimal.NewFromFloat(25.0)
	numGrants := 10

	promotion, err := pg.CreatePromotion("ugp", numGrants, value, "")
	suite.Require().NoError(err, "Create promotion should succeed")

	promotion, err = pg.GetPromotion(promotion.ID)
	suite.Require().NoError(err, "Get promotion should succeed")

	suite.Assert().Equal(numGrants, promotion.RemainingGrants)
	suite.Assert().True(value.Equal(promotion.ApproximateValue))
}

func (suite *PostgresTestSuite) TestActivatePromotion() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	promotion, err := pg.CreatePromotion("ugp", 1, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")

	suite.Assert().False(promotion.Active)

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotion, err = pg.GetPromotion(promotion.ID)
	suite.Require().NoError(err, "Get promotion should succeed")

	suite.Assert().True(promotion.Active)
}

func (suite *PostgresTestSuite) TestInsertIssuer() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	promotion, err := pg.CreatePromotion("ugp", 10, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")

	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "test", PublicKey: publicKey}
	_, err = pg.InsertIssuer(issuer)

	suite.Require().NoError(err, "Save issuer should succeed")
}

func (suite *PostgresTestSuite) TestGetIssuer() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	promotion, err := pg.CreatePromotion("ugp", 10, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")

	origIssuer := &Issuer{PromotionID: promotion.ID, Cohort: "test", PublicKey: publicKey}
	origIssuer, err = pg.InsertIssuer(origIssuer)
	suite.Require().NoError(err, "Insert issuer should succeed")

	issuerByPromoAndCohort, err := pg.GetIssuer(promotion.ID, "test")
	suite.Require().NoError(err, "Get issuer should succeed")
	suite.Assert().Equal(origIssuer, issuerByPromoAndCohort)

	issuerByPublicKey, err := pg.GetIssuerByPublicKey(publicKey)
	suite.Require().NoError(err, "Get issuer by public key should succeed")
	suite.Assert().Equal(origIssuer, issuerByPublicKey)
}

func (suite *PostgresTestSuite) TestCreateClaim() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	promotion, err := pg.CreatePromotion("ads", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	w := &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(walletDB.UpsertWallet(context.Background(), w), "Save wallet should succeed")

	_, err = pg.CreateClaim(promotion.ID, w.ID, decimal.NewFromFloat(30.0), decimal.NewFromFloat(0), false)
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")
}

func (suite *PostgresTestSuite) TestGetPreClaim() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	promotion, err := pg.CreatePromotion("ads", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	w := &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(walletDB.UpsertWallet(context.Background(), w), "Save wallet should succeed")

	expectedClaim, err := pg.CreateClaim(promotion.ID, w.ID, decimal.NewFromFloat(30.0), decimal.NewFromFloat(0), false)
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")

	claim, err := pg.GetPreClaim(promotion.ID, w.ID)
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Assert().Equal(expectedClaim, claim)
}

func (suite *PostgresTestSuite) TestClaimForWallet() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	blindedCreds := jsonutils.JSONStringArray([]string{})

	promotion, err := pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")

	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Require().NoError(err, "Insert issuer should succeed")

	w := &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(walletDB.UpsertWallet(context.Background(), w), "Save wallet should succeed")

	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Require().Error(err, "Claim for wallet should fail, promotion is not active")

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Require().NoError(err, "Claim for wallet should succeed, promotion is active and has grants left")
	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Require().Error(err, "Claim for wallet should fail, wallet already claimed this promotion")

	w = &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(walletDB.UpsertWallet(context.Background(), w), "Save wallet should succeed")
	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Require().NoError(err, "Claim for wallet should succeed, promotion is active and has grants left")

	w = &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(walletDB.UpsertWallet(context.Background(), w), "Save wallet should succeed")
	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Require().Error(err, "Claim for wallet should fail, promotion is active but has no more grants")

	promotion, err = pg.CreatePromotion("ads", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	w = &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(walletDB.UpsertWallet(context.Background(), w), "Save wallet should succeed")

	_, err = pg.CreateClaim(promotion.ID, w.ID, decimal.NewFromFloat(30.0), decimal.NewFromFloat(0), false)
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")

	w2 := &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(walletDB.UpsertWallet(context.Background(), w2), "Save wallet should succeed")
	_, err = pg.ClaimForWallet(promotion, issuer, w2, blindedCreds)
	suite.Require().Error(err, "Claim for wallet should fail, wallet does not have pre-registered claim")

	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Require().NoError(err, "Claim for wallet should succeed, wallet has pre-registered claim")

	promotion, err = pg.GetPromotion(promotion.ID)
	suite.Require().NoError(err, "Get promotion should succeed")
	suite.Assert().Equal(1, promotion.RemainingGrants)
}

func (suite *PostgresTestSuite) TestGetAvailablePromotionsForWallet() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	w := &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(walletDB.UpsertWallet(context.Background(), w), "Save wallet should succeed")

	promotions, err := pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	promotion, err := pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	promotion.PublicKeys = jsonutils.JSONStringArray{}

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Require().NoError(err, "Insert issuer should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().True(promotions[0].Active)
	suite.Assert().True(promotions[0].Available)

	promotion, err = pg.CreatePromotion("ads", 2, decimal.NewFromFloat(35.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	issuer = &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Require().NoError(err, "Insert issuer should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().True(promotions[0].Available)

	// 30.7 * 4 = 122.8 => test differences in rounding
	adClaimValue := decimal.NewFromFloat(30.7)
	claim, err := pg.CreateClaim(promotion.ID, w.ID, adClaimValue, decimal.NewFromFloat(0), false)
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")
	adSuggestionsPerGrant, err := claim.SuggestionsNeeded(promotion)
	suite.Require().NoError(err)

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(2, len(promotions))
	suite.Assert().True(promotions[0].Available)
	suite.Assert().True(promotions[1].Available)
	suite.Assert().True(adClaimValue.Equals(promotions[1].ApproximateValue))
	suite.Assert().Equal(adSuggestionsPerGrant, promotions[1].SuggestionsPerGrant)

	promotion, err = pg.CreatePromotion("ads", 2, decimal.NewFromFloat(35.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	issuer = &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Require().NoError(err, "Insert issuer should succeed")

	// test when claim is for less than the value of one vote
	adClaimValue = decimal.NewFromFloat(0.05)
	claim, err = pg.CreateClaim(promotion.ID, w.ID, adClaimValue, decimal.NewFromFloat(0), false)
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")
	adSuggestionsPerGrant, err = claim.SuggestionsNeeded(promotion)
	suite.Require().NoError(err)

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(3, len(promotions))
	suite.Assert().True(promotions[0].Available)
	suite.Assert().True(promotions[1].Available)
	suite.Assert().True(promotions[2].Available)
	suite.Assert().True(adClaimValue.Equals(promotions[2].ApproximateValue))
	suite.Assert().Equal(adSuggestionsPerGrant, promotions[2].SuggestionsPerGrant)
	suite.Assert().Equal(1, adSuggestionsPerGrant)
}

func (suite *PostgresTestSuite) TestGetAvailablePromotions() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	promotions, err := pg.GetAvailablePromotions("")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	promotion, err := pg.CreatePromotion("ugp", 0, decimal.NewFromFloat(15.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	promotion.PublicKeys = jsonutils.JSONStringArray{}
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotions("")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions), "Active promo with no grants should not appears in legacy list")

	suite.CleanDB()

	promotion, err = pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	promotion.PublicKeys = jsonutils.JSONStringArray{}

	promotions, err = pg.GetAvailablePromotions("")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotions("")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().True(promotions[0].Active)
	suite.Assert().True(promotions[0].Available)

	promotion, err = pg.CreatePromotion("ads", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotions("")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().True(promotions[0].Active)
	suite.Assert().True(promotions[0].Available)

	// Test platform='desktop' returns all desktop grants for non-legacy
	// GetAvailablePromotions endpoint w/o walletID
	suite.CleanDB()

	// Create desktop promotion
	promotion, err = pg.CreatePromotion("ugp", 1, decimal.NewFromFloat(25.0), "desktop")
	suite.Require().NoError(err, "Create promotion should succeed")
	err = pg.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Activate promotion should succeed")

	// Ensure they are all returned
	promotions, err = pg.GetAvailablePromotions("desktop")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	promotions, err = pg.GetAvailablePromotions("osx")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	promotions, err = pg.GetAvailablePromotions("linux")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	promotions, err = pg.GetAvailablePromotions("windows")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	// Test platform='desktop' returns all desktop grants for legacy
	// GetAvailablePromotions endpoint without walletID
	suite.CleanDB()

	promotion, err = pg.CreatePromotion("ugp", 1, decimal.NewFromFloat(25.0), "desktop")
	suite.Require().NoError(err, "Create promotion should succeed")
	err = pg.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Activate promotion should succeed")

	// Ensure they are all returned
	// Legacy endpoints only return active
	err = pg.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotions("desktop")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	promotions, err = pg.GetAvailablePromotions("osx")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	promotions, err = pg.GetAvailablePromotions("linux")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	promotions, err = pg.GetAvailablePromotions("windows")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(len(promotions), 1)

	suite.CleanDB()

	// Create desktop promotion
	promotion, err = pg.CreatePromotion("ugp", 1, decimal.NewFromFloat(25.0), "ios")
	suite.Require().NoError(err, "Create promotion should succeed")

	// it should not be in the list until activated
	promotions, err = pg.GetAvailablePromotions("ios")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	err = pg.ActivatePromotion(promotion)

	promotions, err = pg.GetAvailablePromotions("ios")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))

	// Desktop should not see an iOS grant
	promotions, err = pg.GetAvailablePromotions("desktop")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	// But iOS should
	promotions, err = pg.GetAvailablePromotions("ios")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
}

func (suite *PostgresTestSuite) TestGetAvailablePromotionsForWalletLegacy() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	w := &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(walletDB.UpsertWallet(context.Background(), w), "Save wallet should succeed")
	w2 := &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(walletDB.UpsertWallet(context.Background(), w2), "Save wallet should succeed")

	// create an ancient promotion to make sure no new claims can be made on it
	ancient_promotion, err := pg.CreatePromotion("ugp", 1, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create Promotion should succeed")
	changed, err := pg.RawDB().Exec(`
		update promotions set created_at= NOW() - INTERVAL '4 months' where id=$1`, ancient_promotion.ID)
	suite.Require().NoError(err, "should be able to set the promotion created_at to 4 months ago")
	changed_rows, _ := changed.RowsAffected()
	suite.Assert().Equal(int64(1), changed_rows)

	// at this point the promotion should no longer show up for the wallet, making the list empty below:
	promotions, err := pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions))

	promotion, err := pg.CreatePromotion("ugp", 1, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")

	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Require().NoError(err, "Insert issuer should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions), "Legacy listing should not show inactive promotions")

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions))
	suite.Assert().True(promotions[0].Active)
	suite.Assert().True(promotions[0].Available)

	// Simulate legacy claim
	claim, err := pg.CreateClaim(promotion.ID, w.ID, decimal.NewFromFloat(25.0), decimal.NewFromFloat(0), false)
	suite.Require().NoError(err, "Creating claim should succeed")
	_, err = pg.RawDB().Exec("update claims set legacy_claimed = true where claims.id = $1", claim.ID)
	suite.Require().NoError(err, "Setting legacy_claimed should succeed")
	_, err = pg.RawDB().Exec(`update promotions set remaining_grants = remaining_grants - 1 where id = $1 and active`, promotion.ID)
	suite.Require().NoError(err, "Setting remaining grants should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions), "Legacy claimed promotions should appear in non-legacy list")
	suite.Assert().True(promotions[0].Active)
	suite.Assert().True(promotions[0].Available)

	promotions, err = pg.GetAvailablePromotionsForWallet(w2, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(0, len(promotions), "Promotion with one grant should not appear after one claim")

	promotion, err = pg.CreatePromotion("ads", 1, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	issuer = &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Require().NoError(err, "Insert issuer should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(1, len(promotions), "Unavailable ads promo should not appear")

	// Create pre-registered ads claim
	claim, err = pg.CreateClaim(promotion.ID, w.ID, decimal.NewFromFloat(30.0), decimal.NewFromFloat(0), false)
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(2, len(promotions))
	suite.Assert().True(promotions[1].Available)

	// Simulate legacy claim
	_, err = pg.RawDB().Exec("update claims set legacy_claimed = true where claims.id = $1", claim.ID)
	suite.Require().NoError(err, "Setting legacy_claimed should succeed")
	_, err = pg.RawDB().Exec(`update promotions set remaining_grants = remaining_grants - 1 where id = $1 and active`, promotion.ID)
	suite.Require().NoError(err, "Setting remaining grants should succeed")

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(2, len(promotions), "Legacy claimed promotions should appear in non-legacy list")
	suite.Assert().True(promotions[0].Available)
	suite.Assert().True(promotions[1].Available)

	// Deactivate a promotion
	suite.Require().NoError(pg.DeactivatePromotion(&promotions[0]))

	promotions, err = pg.GetAvailablePromotionsForWallet(w, "")
	suite.Require().NoError(err, "Get promotions should succeed")
	suite.Assert().Equal(2, len(promotions), "Deactivated legacy claimed promotions should appear in the non-legacy list")
}

func (suite *PostgresTestSuite) TestGetClaimCreds() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	blindedCreds := jsonutils.JSONStringArray([]string{"hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="})

	promotion, err := pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")

	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Require().NoError(err, "Insert issuer should succeed")

	w := &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(walletDB.UpsertWallet(context.Background(), w), "Save wallet should succeed")

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	claim, err := pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Require().NoError(err, "Claim for wallet should succeed, promotion is active and has grants left")

	claimCreds, err := pg.GetClaimCreds(claim.ID)
	suite.Require().NoError(err, "Get claim creds should succeed")

	suite.Assert().Equal(blindedCreds, claimCreds.BlindedCreds)
}

func (suite *PostgresTestSuite) TestGetClaimByWalletAndPromotion() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	blindedCreds := jsonutils.JSONStringArray([]string{"hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="})
	w := &walletutils.Info{
		ID:         uuid.NewV4().String(),
		Provider:   "uphold",
		ProviderID: uuid.NewV4().String(),
		PublicKey:  publicKey,
	}
	err = walletDB.UpsertWallet(context.Background(), w)

	// Create promotion
	promotion, err := pg.CreatePromotion(
		"ugp",
		2,
		decimal.NewFromFloat(50.0),
		"",
	)
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Require().NoError(err, "Insert issuer should succeed")

	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Require().NoError(err, "Claim creation should succeed")

	// First try to look up a a claim for a wallet that doesn't have one
	fakeWallet := &walletutils.Info{ID: uuid.NewV4().String()}
	claim, err := pg.GetClaimByWalletAndPromotion(fakeWallet, promotion)
	suite.Require().NoError(err, "Get claim by wallet and promotion should succeed")
	suite.Assert().Nil(claim)

	// Now look up claim for wallet that does have one
	claim, err = pg.GetClaimByWalletAndPromotion(w, promotion)
	suite.Require().NoError(err, "Get claim by wallet and promotion should succeed")
	suite.Assert().Equal(claim.PromotionID, promotion.ID)
	suite.Assert().Equal(claim.WalletID.String(), w.ID)

	promotion, err = pg.CreatePromotion("ads", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	_, err = pg.CreateClaim(promotion.ID, w.ID, decimal.NewFromFloat(30.0), decimal.NewFromFloat(0), false)
	suite.Require().NoError(err, "Creating pre-registered claim should succeed")

	// A preregistered claim should not exist
	claim, err = pg.GetClaimByWalletAndPromotion(w, promotion)
	suite.Require().NoError(err, "Get claim by wallet and promotion should succeed")
	suite.Assert().Nil(claim)
}

func (suite *PostgresTestSuite) TestSaveClaimCreds() {
	// FIXME
}

func (suite *PostgresTestSuite) TestRunNextClaimJob() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	mockClaimWorker := NewMockClaimWorker(mockCtrl)

	attempted, err := pg.RunNextClaimJob(context.Background(), mockClaimWorker)
	suite.Assert().Equal(false, attempted)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	blindedCreds := jsonutils.JSONStringArray([]string{"hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="})
	signedCreds := jsonutils.JSONStringArray([]string{"hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="})
	batchProof := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	promotion, err := pg.CreatePromotion("ugp", 2, decimal.NewFromFloat(25.0), "")
	suite.Require().NoError(err, "Create promotion should succeed")

	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Require().NoError(err, "Insert issuer should succeed")

	w := &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(walletDB.UpsertWallet(context.Background(), w), "Save wallet should succeed")

	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	claim, err := pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Require().NoError(err, "Claim for wallet should succeed, promotion is active and has grants left")

	creds := &ClaimCreds{
		ID:           claim.ID,
		BlindedCreds: blindedCreds,
		SignedCreds:  &signedCreds,
		BatchProof:   &batchProof,
		PublicKey:    &issuer.PublicKey,
	}

	// One signing job should run
	mockClaimWorker.EXPECT().SignClaimCreds(gomock.Any(), gomock.Eq(claim.ID), gomock.Eq(*issuer), gomock.Eq([]string(blindedCreds))).Return(nil, errors.New("Worker failed"))
	attempted, err = pg.RunNextClaimJob(context.Background(), mockClaimWorker)
	suite.Assert().Equal(true, attempted)
	suite.Require().Error(err)

	// Signing job should rerun on failure
	mockClaimWorker.EXPECT().SignClaimCreds(gomock.Any(), gomock.Eq(claim.ID), gomock.Eq(*issuer), gomock.Eq([]string(blindedCreds))).Return(creds, nil)
	attempted, err = pg.RunNextClaimJob(context.Background(), mockClaimWorker)
	suite.Assert().Equal(true, attempted)
	suite.Require().NoError(err)

	// No further jobs should run after success
	attempted, err = pg.RunNextClaimJob(context.Background(), mockClaimWorker)
	suite.Assert().Equal(false, attempted)
	suite.Require().NoError(err)
}

func (suite *PostgresTestSuite) TestInsertClobberedClaims() {
	ctx := context.Background()
	id1 := uuid.NewV4()
	id2 := uuid.NewV4()

	pg, _, err := NewPostgres()
	suite.Assert().NoError(err)
	suite.Require().NoError(pg.InsertClobberedClaims(ctx, []uuid.UUID{id1, id2}, 1), "Create promotion should succeed")

	var allCreds1 []ClobberedCreds
	var allCreds2 []ClobberedCreds
	err = pg.RawDB().Select(&allCreds1, `select * from clobbered_claims;`)
	suite.Require().NoError(err, "selecting the clobbered creds ids should not result in an error")

	suite.Require().NoError(pg.InsertClobberedClaims(ctx, []uuid.UUID{id1, id2}, 1), "Create promotion should succeed")
	err = pg.RawDB().Select(&allCreds2, `select * from clobbered_claims;`)
	suite.Require().NoError(err, "selecting the clobbered creds ids should not result in an error")
	suite.Assert().Equal(allCreds1, allCreds2, "creds should not be inserted more than once")
}

//go:build integration

package promotion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/clients/gemini"
	"github.com/brave-intl/bat-go/libs/custodian"

	"github.com/jmoiron/sqlx"

	"github.com/brave-intl/bat-go/libs/ptr"

	"github.com/brave-intl/bat-go/libs/logging"

	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"

	"github.com/brave-intl/bat-go/libs/clients/cbr"
	"github.com/brave-intl/bat-go/libs/jsonutils"
	testutils "github.com/brave-intl/bat-go/libs/test"
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

func (suite *PostgresTestSuite) TestDrainClaimErred() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	blindedCreds := jsonutils.JSONStringArray([]string{"hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="})
	walletID := uuid.NewV4()
	wallet2ID := uuid.NewV4()
	info := &walletutils.Info{
		ID:         walletID.String(),
		Provider:   "uphold",
		ProviderID: uuid.NewV4().String(),
		PublicKey:  publicKey,
	}
	info2 := &walletutils.Info{
		ID:         wallet2ID.String(),
		Provider:   "uphold",
		ProviderID: uuid.NewV4().String(),
		PublicKey:  publicKey,
	}
	err = walletDB.UpsertWallet(context.Background(), info)
	err = walletDB.UpsertWallet(context.Background(), info2)
	suite.Require().NoError(err, "Upsert wallet must succeed")

	{
		tmp := uuid.NewV4()
		info.AnonymousAddress = &tmp
	}
	err = walletDB.UpsertWallet(context.Background(), info)
	suite.Require().NoError(err, "Upsert wallet should succeed")

	wallet, err := walletDB.GetWallet(context.Background(), walletID)
	suite.Require().NoError(err, "Get wallet should succeed")
	suite.Assert().Equal(wallet.AnonymousAddress, info.AnonymousAddress)

	wallet2, err := walletDB.GetWallet(context.Background(), wallet2ID)
	suite.Require().NoError(err, "Get wallet should succeed")

	total := decimal.NewFromFloat(50.0)
	// Create promotion
	promotion, err := pg.CreatePromotion(
		"ugp",
		2,
		total,
		"",
	)
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Require().NoError(err, "Insert issuer should succeed")

	claim, err := pg.ClaimForWallet(promotion, issuer, info, blindedCreds)
	suite.Require().NoError(err, "Claim creation should succeed")

	suite.Assert().Equal(false, claim.Drained)

	credentials := []cbr.CredentialRedemption{}

	drainID := uuid.NewV4()

	err = pg.DrainClaim(&drainID, claim, credentials, wallet2, total, errMismatchedWallet)
	suite.Require().NoError(err, "Drain claim errored call should succeed")

	// should show as drained
	claim, err = pg.GetClaimByWalletAndPromotion(wallet, promotion)
	suite.Assert().Equal(true, claim.Drained)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	mockDrainWorker := NewMockDrainWorker(mockCtrl)

	// After err no further job should run
	attempted, err := pg.RunNextDrainJob(context.Background(), mockDrainWorker)
	suite.Assert().Equal(false, attempted)
	suite.Require().NoError(err)

}

func (suite *PostgresTestSuite) TestDrainClaim() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	blindedCreds := jsonutils.JSONStringArray([]string{"hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="})
	walletID := uuid.NewV4()
	info := &walletutils.Info{
		ID:         walletID.String(),
		Provider:   "uphold",
		ProviderID: uuid.NewV4().String(),
		PublicKey:  publicKey,
	}
	err = walletDB.UpsertWallet(context.Background(), info)
	suite.Require().NoError(err, "Upsert wallet must succeed")

	{
		tmp := uuid.NewV4()
		info.AnonymousAddress = &tmp
	}
	err = walletDB.UpsertWallet(context.Background(), info)
	suite.Require().NoError(err, "Upsert wallet should succeed")

	wallet, err := walletDB.GetWallet(context.Background(), walletID)
	suite.Require().NoError(err, "Get wallet should succeed")
	suite.Assert().Equal(wallet.AnonymousAddress, info.AnonymousAddress)

	total := decimal.NewFromFloat(50.0)
	// Create promotion
	promotion, err := pg.CreatePromotion(
		"ugp",
		2,
		total,
		"",
	)
	suite.Require().NoError(err, "Create promotion should succeed")
	suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")

	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = pg.InsertIssuer(issuer)
	suite.Require().NoError(err, "Insert issuer should succeed")

	claim, err := pg.ClaimForWallet(promotion, issuer, info, blindedCreds)
	suite.Require().NoError(err, "Claim creation should succeed")

	suite.Assert().Equal(false, claim.Drained)

	credentials := []cbr.CredentialRedemption{}

	drainID := uuid.NewV4()

	err = pg.DrainClaim(&drainID, claim, credentials, wallet, total, nil)
	suite.Require().NoError(err, "Drain claim should succeed")

	claim, err = pg.GetClaimByWalletAndPromotion(wallet, promotion)
	suite.Assert().Equal(true, claim.Drained)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	mockDrainWorker := NewMockDrainWorker(mockCtrl)

	// One drain job should run
	mockDrainWorker.EXPECT().RedeemAndTransferFunds(gomock.Any(), gomock.Eq(credentials), gomock.Any()).Return(nil, errors.New("Worker failed"))
	attempted, err := pg.RunNextDrainJob(context.Background(), mockDrainWorker)
	suite.Assert().Equal(true, attempted)
	suite.Require().Error(err)

	// After err no further job should run
	attempted, err = pg.RunNextDrainJob(context.Background(), mockDrainWorker)
	suite.Assert().Equal(false, attempted)
	suite.Require().NoError(err)

	// FIXME add test for successful drain job
}

func (suite *PostgresTestSuite) TestDrainClaims_Success() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	walletInfo := walletutils.Info{
		ID:       uuid.NewV4().String(),
		Provider: "uphold",
	}

	err = walletDB.UpsertWallet(context.Background(), &walletInfo)
	suite.NoError(err)

	drainClaims := make([]DrainClaim, 5)
	for i := 0; i < 5; i++ {
		total := decimal.NewFromFloat(rand.Float64())

		promotion, err := pg.CreatePromotion("ugp", 1,
			decimal.NewFromFloat(1), testutils.RandomString())
		suite.Require().NoError(err)

		claim, err := pg.CreateClaim(promotion.ID, walletInfo.ID, total, decimal.NewFromFloat(0), false)
		suite.Require().NoError(err)

		credentialRedemptions := []cbr.CredentialRedemption{
			{
				Issuer:        testutils.RandomString(),
				TokenPreimage: testutils.RandomString(),
				Signature:     testutils.RandomString(),
			},
		}

		drainClaims[i] = DrainClaim{
			BatchID:     ptr.FromUUID(uuid.NewV4()),
			Claim:       claim,
			Credentials: credentialRedemptions,
			Wallet:      &walletInfo,
			Total:       total,
			CodedErr:    nil,
		}
	}

	err = pg.DrainClaims(drainClaims)

	// assert correct number of claims and claims drains inserted

	var claims []Claim
	err = pg.RawDB().Select(&claims, "SELECT * FROM claims")
	suite.Require().NoError(err)
	suite.Require().Equal(len(drainClaims), len(claims))

	var claimDrains []DrainJob
	err = pg.RawDB().Select(&claimDrains, "SELECT * FROM claim_drain")
	suite.Require().NoError(err)
	suite.Require().Equal(len(drainClaims), len(claimDrains))

	// assert the retrieved claims and claims drains inserted are the ones added

	sort.Slice(drainClaims, func(i, j int) bool {
		return drainClaims[i].Claim.ID.String() < drainClaims[j].Claim.ID.String()
	})

	sort.Slice(claims, func(i, j int) bool {
		return claims[i].ID.String() < claims[j].ID.String()
	})

	sort.Slice(claimDrains, func(i, j int) bool {
		return claimDrains[i].ClaimID.String() < claimDrains[j].ClaimID.String()
	})

	for i := 0; i < 5; i++ {
		suite.Require().Equal(drainClaims[i].Claim.ID.String(), claims[i].ID.String())
		suite.Require().Equal(drainClaims[i].Claim.ID.String(), claimDrains[i].ClaimID.String())
	}
}

func (suite *PostgresTestSuite) TestRunNextDrainJob_Gemini_Claim() {
	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	drainJob := suite.insertClaimDrainWithStatus(pg, "", false)

	transactionInfo := walletutils.TransactionInfo{}
	transactionInfo.Status = txnStatusGeminiPending

	drainWorker := NewMockDrainWorker(ctrl)
	drainWorker.EXPECT().
		RedeemAndTransferFunds(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&transactionInfo, nil)

	attempted, err := pg.RunNextDrainJob(context.Background(), drainWorker)
	suite.True(attempted)

	// get the updated drain job and assert
	err = pg.RawDB().Get(&drainJob, "select * from claim_drain where id = $1", drainJob.ID)
	suite.Require().NoError(err)

	suite.Require().Equal(txnStatusGeminiPending, *drainJob.Status)
	suite.Require().Equal(false, drainJob.Erred)
	suite.Require().Nil(drainJob.ErrCode)
}

func (suite *PostgresTestSuite) TestDrainRetryJob_Success() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	walletID := uuid.NewV4()

	query := `INSERT INTO claim_drain (wallet_id, erred, errcode, status, batch_id, credentials, completed, total) 
				VALUES ($1, $2, $3, $4, $5, '[{"t":"123"}]', FALSE, 1);`

	_, err = pg.RawDB().ExecContext(context.Background(), query, walletID.String(), true, "reputation-failed", "reputation-failed",
		uuid.NewV4().String())
	suite.Require().NoError(err, "should have inserted claim drain row")

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	drainRetryWorker := NewMockDrainRetryWorker(ctrl)
	drainRetryWorker.EXPECT().
		FetchAdminAttestationWalletID(gomock.Eq(ctx)).
		Return(&walletID, nil).
		AnyTimes()

	go func(ctx2 context.Context) {
		pg.RunNextDrainRetryJob(ctx2, drainRetryWorker)
	}(ctx)

	time.Sleep(1 * time.Millisecond)

	var drainJob DrainJob
	err = pg.RawDB().Get(&drainJob, `SELECT * FROM claim_drain WHERE wallet_id = $1 LIMIT 1`, walletID)
	suite.Require().NoError(err, "should have retrieved drain job")

	suite.Require().Equal(walletID, drainJob.WalletID)
	suite.Require().Equal(false, drainJob.Erred)
	suite.Require().Equal("reputation-failed", *drainJob.ErrCode)
	suite.Require().Equal("retry-bypass-cbr", *drainJob.Status)
}

func (suite *PostgresTestSuite) TestRunNextBatchPaymentsJob_NoClaimsToProcess() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	batchTransferWorker := NewMockBatchTransferWorker(ctrl)

	actual, err := pg.RunNextBatchPaymentsJob(context.Background(), batchTransferWorker)
	suite.Require().NoError(err)
	suite.Require().False(actual, "should not have attempted job run")
}

func (suite *PostgresTestSuite) TestRunNextBatchPaymentsJob_SubmitBatchTransfer_Error() {
	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	ctx, _ := logging.SetupLogger(context.Background())

	// setup wallet
	walletID := uuid.NewV4()
	userDepositAccountProvider := "bitflyer"

	info := &walletutils.Info{
		ID:                         walletID.String(),
		Provider:                   "uphold",
		ProviderID:                 uuid.NewV4().String(),
		PublicKey:                  "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY=",
		UserDepositAccountProvider: &userDepositAccountProvider,
	}
	err = walletDB.UpsertWallet(ctx, info)
	suite.Require().NoError(err)

	// setup claim drain
	batchID := uuid.NewV4()

	query := `INSERT INTO claim_drain (wallet_id, erred, errcode, status, batch_id, credentials, completed, total, transaction_id) 
				VALUES ($1, $2, $3, $4, $5, '[{"t":123}]', FALSE, 1, $6);`

	_, err = pg.RawDB().ExecContext(context.Background(), query, walletID, false, nil, "prepared", batchID, uuid.NewV4().String())
	suite.Require().NoError(err, "should have inserted claim drain row")

	drainCodeErr := drainCodeErrorInvalidDepositID
	drainCodeError := errorutils.New(errors.New("error-text"),
		"error-message", drainCodeErr)

	batchTransferWorker := NewMockBatchTransferWorker(ctrl)
	batchTransferWorker.EXPECT().
		SubmitBatchTransfer(ctx, &batchID).
		Return(drainCodeError)

	actual, actualErr := pg.RunNextBatchPaymentsJob(ctx, batchTransferWorker)

	var drainJob DrainJob
	err = pg.RawDB().Get(&drainJob, `SELECT * FROM claim_drain WHERE wallet_id = $1 LIMIT 1`, walletID)
	suite.Require().NoError(err, "should have retrieved drain job")

	suite.Require().Equal(walletID, drainJob.WalletID)
	suite.Require().Equal(true, drainJob.Erred)
	suite.Require().Equal(drainCodeErr.ErrCode, *drainJob.ErrCode)
	suite.Require().Equal("failed", *drainJob.Status)

	suite.Require().True(actual, "should have attempted job run")
	suite.Require().Equal(drainCodeError, actualErr)
}

// tests batches are only processed once the drain job has set all claims drains
// to prepared and have they have transactionIDs
func (suite *PostgresTestSuite) TestRunNextBatchPaymentsJob_NextDrainJob_Concurrent() {

	suite.CleanDB()

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err)

	ctx, cancel := context.WithCancel(context.Background())

	walletID := uuid.NewV4()
	batchID := uuid.NewV4()

	info := &walletutils.Info{
		ID:                         walletID.String(),
		Provider:                   "uphold",
		ProviderID:                 uuid.NewV4().String(),
		PublicKey:                  "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY=",
		UserDepositAccountProvider: ptr.FromString("bitflyer"),
	}
	err = walletDB.UpsertWallet(ctx, info)
	suite.Require().NoError(err)

	// setup 3 claim drains and 1 erred claim drain in batch
	for i := 0; i < 3; i++ {
		claimDrainFixtures(pg.RawDB(), batchID, walletID, false, false)
	}
	// setup one erred job as part of batch which should not get run by batch payment
	claimDrainFixtures(pg.RawDB(), batchID, walletID, false, true)

	transactionInfo := walletutils.TransactionInfo{}
	transactionInfo.Status = "bitflyer-consolidate"
	transactionInfo.ID = uuid.NewV4().String()

	drainWorker := NewMockDrainWorker(ctrl)
	drainWorker.EXPECT().
		RedeemAndTransferFunds(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&transactionInfo, nil).
		Times(3)

	batchTransferWorker := NewMockBatchTransferWorker(ctrl)
	batchTransferWorker.EXPECT().
		SubmitBatchTransfer(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	// start batch payments job to pick up claim drains when all in batch are in prepared state
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				pg.RunNextBatchPaymentsJob(context.Background(), batchTransferWorker)
			}
		}
	}(ctx)

	// run next drain job to pickup job and set as prepared and set transactionID

	attempted, err := pg.RunNextDrainJob(ctx, drainWorker)
	suite.Require().True(attempted)
	suite.NoError(err)

	time.Sleep(200 * time.Millisecond)

	var drainJobsFirst []DrainJob
	err = pg.RawDB().Select(&drainJobsFirst, `SELECT * FROM claim_drain`)
	suite.Require().NoError(err, "should have retrieved drain job")

	// check no jobs have been submitted to batch and one job are prepared
	prepared := 0
	for _, drain := range drainJobsFirst {
		suite.Require().NotEqual("submitted", ptr.String(drain.Status),
			fmt.Sprintf("should not be submitted got %s", ptr.String(drain.Status)))
		if ptr.String(drain.Status) == "prepared" {
			prepared += 1
		}
	}
	suite.Require().Equal(1, prepared)

	// run next drain job to pickup job and set as prepared and set transactionID

	attempted, err = pg.RunNextDrainJob(ctx, drainWorker)
	suite.Require().True(attempted)
	suite.NoError(err)

	time.Sleep(200 * time.Millisecond)

	var drainJobsSecond []DrainJob
	err = pg.RawDB().Select(&drainJobsSecond, `SELECT * FROM claim_drain`)
	suite.Require().NoError(err, "should have retrieved drain job")

	// check no jobs have been submitted to batch and two job are prepared
	prepared = 0
	for _, drain := range drainJobsSecond {
		suite.Require().NotEqual("submitted", ptr.String(drain.Status),
			fmt.Sprintf("should not be submitted got %s", ptr.String(drain.Status)))
		if ptr.String(drain.Status) == "prepared" {
			prepared += 1
		}
	}
	suite.Require().Equal(2, prepared)

	// run final next drain job to set claim drain as prepared and transactionID

	attempted, err = pg.RunNextDrainJob(ctx, drainWorker)
	suite.Require().True(attempted)
	suite.NoError(err)

	time.Sleep(200 * time.Millisecond)

	// run batch payments should now pickup all claims in batch and process
	var actual []DrainJob
	err = pg.RawDB().Select(&actual, `SELECT * FROM claim_drain`)
	suite.Require().NoError(err, "should have retrieved drain job")

	suite.Require().Equal(4, len(actual))

	// assert we have 3 submitted and 1 erred claim drain
	submitted := 0
	erred := 0
	for _, drain := range actual {
		// submitted
		if !drain.Erred {
			submitted += 1
			suite.Require().Equal("submitted", ptr.String(drain.Status),
				fmt.Sprintf("should be submitted got %s", ptr.String(drain.Status)))
			suite.Require().Equal(batchID, *drain.BatchID)
			suite.Require().NotNil(drain.TransactionID)
		}
		// erred
		if drain.Erred {
			erred += 1
			suite.Require().Equal(batchID, *drain.BatchID)
			suite.Require().Nil(drain.TransactionID)
		}
	}
	suite.Require().Equal(3, submitted)
	suite.Require().Equal(1, erred)

	// shutdown bath payments job routine
	cancel()
}

func (suite *PostgresTestSuite) TestUpdateDrainJobAsRetriable_Success() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletID := uuid.NewV4()

	query := `INSERT INTO claim_drain (wallet_id, erred, errcode, status, batch_id, credentials, completed, total) 
				VALUES ($1, $2, $3, $4, $5, '[{"t":"123"}]', FALSE, 1);`

	_, err = pg.RawDB().ExecContext(context.Background(), query, walletID, true, "some-failed-errcode", "failed",
		uuid.NewV4().String())
	suite.Require().NoError(err, "should have inserted claim drain row")

	err = pg.UpdateDrainJobAsRetriable(context.Background(), walletID)
	suite.Require().NoError(err, "should have updated claim drain row")

	var drainJob DrainJob
	err = pg.RawDB().Get(&drainJob, `SELECT * FROM claim_drain WHERE wallet_id = $1 LIMIT 1`, walletID)
	suite.Require().NoError(err, "should have retrieved drain job")

	suite.Require().Equal(walletID, drainJob.WalletID)
	suite.Require().Equal(false, drainJob.Erred)
	suite.Require().Equal("manual-retry", *drainJob.Status)
}

func (suite *PostgresTestSuite) TestUpdateDrainJobAsRetriable_NotFound_WalletID() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	query := `INSERT INTO claim_drain (wallet_id, erred, errcode, status, batch_id, credentials, completed, total) 
				VALUES ($1, $2, $3, $4, $5, '[{"t":"123"}]', FALSE, 1);`

	_, err = pg.RawDB().ExecContext(context.Background(), query, uuid.NewV4(), true, "some-failed-errcode", "failed",
		uuid.NewV4().String())
	suite.Require().NoError(err, "should have inserted claim drain row")

	walletID := uuid.NewV4()
	err = pg.UpdateDrainJobAsRetriable(context.Background(), walletID)

	expected := fmt.Errorf("update drain job: failed to update row for walletID %s: %w", walletID,
		errorutils.ErrNotFound)

	suite.Require().Error(err, expected.Error())
}

func (suite *PostgresTestSuite) TestUpdateDrainJobAsRetriable_NoRetriableJobFound() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	query := `INSERT INTO claim_drain (wallet_id, erred, errcode, status, batch_id, credentials, completed, total) 
				VALUES ($1, $2, $3, $4, $5, '[{"t":"123"}]', FALSE, 1);`

	walletID := uuid.NewV4()

	_, err = pg.RawDB().ExecContext(context.Background(), query, walletID, true, "some-errcode", "complete",
		uuid.NewV4())
	suite.Require().NoError(err, "should have inserted claim drain row")

	err = pg.UpdateDrainJobAsRetriable(context.Background(), walletID)

	expected := fmt.Errorf("update drain job: failed to update row for walletID %s: %w", walletID,
		errorutils.ErrNotFound)

	suite.Require().Error(err, expected.Error())
}

func (suite *PostgresTestSuite) TestUpdateDrainJobAsRetriable_NotFound_Erred() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	query := `INSERT INTO claim_drain (wallet_id, erred, errcode, status, batch_id, credentials, completed, total) 
				VALUES ($1, $2, $3, $4, $5, '[{"t":"123"}]', FALSE, 1);`

	walletID := uuid.NewV4()
	erred := false

	_, err = pg.RawDB().ExecContext(context.Background(), query, walletID, erred, "some-failed-errcode", "failed",
		uuid.NewV4())
	suite.Require().NoError(err, "should have inserted claim drain row")

	err = pg.UpdateDrainJobAsRetriable(context.Background(), walletID)

	expected := fmt.Errorf("update drain job: failed to update row for walletID %s: %w", walletID,
		errorutils.ErrNotFound)

	suite.Require().Error(err, expected.Error())
}

func (suite *PostgresTestSuite) TestUpdateDrainJobAsRetriable_NotFound_TransactionID() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	query := `INSERT INTO claim_drain (wallet_id, erred, errcode, status, batch_id, credentials, completed, total, transaction_id) 
				VALUES ($1, $2, $3, $4, $5, '[{"t":"123"}]', FALSE, 1, $6);`

	walletID := uuid.NewV4()

	_, err = pg.RawDB().ExecContext(context.Background(), query, walletID, true, "some-failed-errcode", "failed",
		uuid.NewV4(), uuid.NewV4())
	suite.Require().NoError(err, "should have inserted claim drain row")

	err = pg.UpdateDrainJobAsRetriable(context.Background(), walletID)

	expected := fmt.Errorf("update drain job: failed to update row for walletID %s: %w", walletID,
		errorutils.ErrNotFound)

	suite.Require().Error(err, expected.Error())
}

func (suite *PostgresTestSuite) TestRunNextDrainJob_CBRBypass_ManualRetry() {
	// clean db so only one claim drain job selectable
	suite.CleanDB()

	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	walletID := uuid.NewV4()

	credentialRedemption := cbr.CredentialRedemption{
		Issuer:        testutils.RandomString(),
		TokenPreimage: testutils.RandomString(),
		Signature:     testutils.RandomString(),
	}
	credentialRedemptions := make([]cbr.CredentialRedemption, 0)
	credentialRedemptions = append(credentialRedemptions, credentialRedemption)

	credentials, err := json.Marshal(credentialRedemptions)
	suite.Require().NoError(err, "should have serialised credentials")

	query := `INSERT INTO claim_drain (wallet_id, erred, errcode, status, batch_id, credentials, completed, total) 
				VALUES ($1, FALSE, 'some-errcode', 'manual-retry', $2, $3, FALSE, 1);`

	_, err = pg.RawDB().ExecContext(context.Background(), query, walletID, uuid.NewV4().String(), credentials)
	suite.Require().NoError(err, "should have inserted claim drain row")

	// expected context with bypass cbr true
	ctrl := gomock.NewController(suite.T())
	drainWorker := NewMockDrainWorker(ctrl)

	ctx := context.Background()

	drainWorker.EXPECT().
		RedeemAndTransferFunds(isCBRBypass(ctx), credentialRedemptions, gomock.Any()).
		Return(&walletutils.TransactionInfo{}, nil)

	attempted, err := pg.RunNextDrainJob(ctx, drainWorker)

	suite.Require().NoError(err, "should have been successful attempted job")
	suite.Require().True(attempted)
}

func (suite *PostgresTestSuite) TestRunNextGeminiCheckStatus_Complete() {
	// clean db so only one claim drain job selectable
	suite.CleanDB()

	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	ctx := context.Background()

	drainJob := suite.insertClaimDrainWithStatus(pg, txnStatusGeminiPending, true)

	// create tx_ref
	settlementTx := custodian.Transaction{
		SettlementID: ptr.String(drainJob.TransactionID),
		Type:         "drain",
		Destination:  ptr.String(drainJob.DepositDestination),
		Channel:      "wallet",
	}
	txRef := gemini.GenerateTxRef(&settlementTx)

	txnStatus := &walletutils.TransactionInfo{Status: "complete"}

	ctrl := gomock.NewController(suite.T())
	geminiTxnStatusWorker := NewMockGeminiTxnStatusWorker(ctrl)
	geminiTxnStatusWorker.EXPECT().
		GetGeminiTxnStatus(ctx, txRef).
		Return(txnStatus, nil)

	attempted, err := pg.RunNextGeminiCheckStatus(ctx, geminiTxnStatusWorker)
	suite.Require().NoError(err, "should be no error")
	suite.Require().True(attempted)

	err = pg.RawDB().Get(&drainJob, "select * from claim_drain where id = $1", drainJob.ID)
	suite.Require().NoError(err)

	suite.Require().Equal("complete", *drainJob.Status)
	suite.Require().True(drainJob.Completed)
	suite.Require().False(drainJob.Erred)
}

func (suite *PostgresTestSuite) TestRunNextGeminiCheckStatus_Pending() {
	// clean db so only one claim drain job selectable
	suite.CleanDB()

	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	ctrl := gomock.NewController(suite.T())
	geminiTxnStatusWorker := NewMockGeminiTxnStatusWorker(ctrl)

	ctx := context.Background()
	txnStatus := &walletutils.TransactionInfo{Status: "pending"}

	// insert drain jobs in pending state and setup mock call to get status
	drainJobs := [5]DrainJob{}
	for i := 0; i < len(drainJobs); i++ {
		drainJob := suite.insertClaimDrainWithStatus(pg, txnStatusGeminiPending, true)

		// create tx_ref
		settlementTx := custodian.Transaction{
			SettlementID: ptr.String(drainJob.TransactionID),
			Type:         "drain",
			Destination:  ptr.String(drainJob.DepositDestination),
			Channel:      "wallet",
		}
		txRef := gemini.GenerateTxRef(&settlementTx)

		geminiTxnStatusWorker.EXPECT().
			GetGeminiTxnStatus(ctx, txRef).
			Return(txnStatus, nil).
			Times(1)

		drainJobs[i] = drainJob
	}

	// check all claim drains are processed in the order they were inserted earliest to latest date
	for i := 0; i < len(drainJobs); i++ {
		attempted, err := pg.RunNextGeminiCheckStatus(ctx, geminiTxnStatusWorker)
		suite.Require().NoError(err, "should be no error")
		suite.Require().True(attempted)

		err = pg.RawDB().Get(&drainJobs[i], "select * from claim_drain where id = $1", drainJobs[i].ID)
		suite.Require().NoError(err)

		suite.Require().Equal(txnStatusGeminiPending, *drainJobs[i].Status)
		suite.Require().False(drainJobs[i].Completed)
		suite.Require().False(drainJobs[i].Erred)
	}

	// should return no jobs as we wait 10 mins before retrying
	attempted, err := pg.RunNextGeminiCheckStatus(ctx, geminiTxnStatusWorker)
	suite.Require().NoError(err, "should be no error")
	suite.Require().False(attempted)

	// retrieve next job to run this should be the first inserted job in the cycle
	var nextDrainJob DrainJob
	err = pg.RawDB().Get(&nextDrainJob, "select * from claim_drain order by updated_at asc limit 1")

	suite.Require().NoError(err)
	suite.Require().Equal(drainJobs[0].ID, nextDrainJob.ID, "should have been first job we inserted")
}

func (suite *PostgresTestSuite) TestRunNextGeminiCheckStatus_Failure() {
	// clean db so only one claim drain job selectable
	suite.CleanDB()

	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	ctx := context.Background()

	drainJob := suite.insertClaimDrainWithStatus(pg, txnStatusGeminiPending, true)

	// create tx_ref
	settlementTx := custodian.Transaction{
		SettlementID: ptr.String(drainJob.TransactionID),
		Type:         "drain",
		Destination:  ptr.String(drainJob.DepositDestination),
		Channel:      "wallet",
	}
	txRef := gemini.GenerateTxRef(&settlementTx)

	note := testutils.RandomString()
	txnStatus := &walletutils.TransactionInfo{Status: "failed", Note: note}

	ctrl := gomock.NewController(suite.T())
	geminiTxnStatusWorker := NewMockGeminiTxnStatusWorker(ctrl)
	geminiTxnStatusWorker.EXPECT().
		GetGeminiTxnStatus(ctx, txRef).
		Return(txnStatus, nil)

	attempted, err := pg.RunNextGeminiCheckStatus(ctx, geminiTxnStatusWorker)
	suite.Require().NoError(err, "should be no error")
	suite.Require().True(attempted)

	err = pg.RawDB().Get(&drainJob, "select * from claim_drain where id = $1", drainJob.ID)
	suite.Require().NoError(err)

	suite.Require().Equal("failed", *drainJob.Status)
	suite.Require().Equal(true, drainJob.Erred)
	suite.Require().Equal(note, *drainJob.ErrCode)
}

func (suite *PostgresTestSuite) TestRunNextGeminiCheckStatus_GetGeminiTxnStatus_Error() {
	// clean db so only one claim drain job selectable
	suite.CleanDB()

	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	ctx := context.Background()

	drainJob := suite.insertClaimDrainWithStatus(pg, txnStatusGeminiPending, true)

	// create tx_ref
	settlementTx := custodian.Transaction{
		SettlementID: ptr.String(drainJob.TransactionID),
		Type:         "drain",
		Destination:  ptr.String(drainJob.DepositDestination),
		Channel:      "wallet",
	}
	txRef := gemini.GenerateTxRef(&settlementTx)

	getGeminiTxnStatusError := fmt.Errorf(testutils.RandomString())

	ctrl := gomock.NewController(suite.T())
	geminiTxnStatusWorker := NewMockGeminiTxnStatusWorker(ctrl)
	geminiTxnStatusWorker.EXPECT().
		GetGeminiTxnStatus(ctx, txRef).
		Return(nil, getGeminiTxnStatusError)

	attempted, err := pg.RunNextGeminiCheckStatus(ctx, geminiTxnStatusWorker)

	suite.Require().Errorf(err, fmt.Sprintf("failed to get status for txn %s: %s",
		*drainJob.TransactionID, getGeminiTxnStatusError.Error()))

	suite.Require().True(attempted)

	err = pg.RawDB().Get(&drainJob, "select * from claim_drain where id = $1", drainJob.ID)
	suite.Require().NoError(err)

	suite.Require().Equal(txnStatusGeminiPending, *drainJob.Status)
	suite.Require().Equal(false, drainJob.Erred)
	suite.Require().Nil(drainJob.ErrCode)
}

func (suite *PostgresTestSuite) TestGetDrainPoll() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	walletID := uuid.NewV4()

	completeID := uuid.NewV4()
	delayedID := uuid.NewV4()
	pendingID := uuid.NewV4()
	inprogressID := uuid.NewV4()

	err = claimDrainFixtures(pg.RawDB(), completeID, walletID, true, false)
	suite.Require().NoError(err, "failed to fixture claim_drain")

	err = claimDrainFixtures(pg.RawDB(), delayedID, walletID, false, true)
	suite.Require().NoError(err, "failed to fixture claim_drain")

	err = claimDrainFixtures(pg.RawDB(), pendingID, walletID, false, false)
	suite.Require().NoError(err, "failed to fixture claim_drain")

	err = claimDrainFixtures(pg.RawDB(), inprogressID, walletID, true, false)
	suite.Require().NoError(err, "failed to fixture claim_drain")

	err = claimDrainFixtures(pg.RawDB(), inprogressID, walletID, false, false)
	suite.Require().NoError(err, "failed to fixture claim_drain")

	service := &Service{
		Datastore: pg,
	}

	drainPoll, err := service.Datastore.GetDrainPoll(&completeID)
	suite.Require().NoError(err, "Failed to get drain poll response")
	suite.Require().True(drainPoll.Status == "complete")

	drainPoll, err = service.Datastore.GetDrainPoll(&delayedID)
	suite.Require().NoError(err, "Failed to get drain poll response")
	suite.Require().True(drainPoll.Status == "delayed")

	drainPoll, err = service.Datastore.GetDrainPoll(&pendingID)
	suite.Require().NoError(err, "Failed to get drain poll response")
	suite.Require().True(drainPoll.Status == "pending")

	drainPoll, err = service.Datastore.GetDrainPoll(&inprogressID)
	suite.Require().NoError(err, "Failed to get drain poll response")
	suite.Require().True(drainPoll.Status == "in_progress")

	// unknown batch_id
	unknownID := uuid.NewV4()
	drainPoll, err = service.Datastore.GetDrainPoll(&unknownID)
	suite.Require().NoError(err, "Failed to get drain poll response")

	suite.Require().True(drainPoll.Status == "unknown")
}

func claimDrainFixtures(db *sqlx.DB, batchID, walletID uuid.UUID, completed, erred bool) error {
	_, err := db.Exec(`INSERT INTO claim_drain (batch_id, credentials, completed, erred, wallet_id, total, updated_at) 
		values ($1, '[{"t":"123"}]', $2, $3, $4, $5, CURRENT_TIMESTAMP);`, batchID, completed, erred, walletID, 1)
	return err
}

func (suite *PostgresTestSuite) insertClaimDrainWithStatus(pg Datastore, status string, hasTransaction bool) DrainJob {
	walletID := uuid.NewV4()

	var transactionID *uuid.UUID
	if hasTransaction {
		transactionID = ptr.FromUUID(uuid.NewV4())
	}

	credentialRedemption := cbr.CredentialRedemption{
		Issuer:        testutils.RandomString(),
		TokenPreimage: testutils.RandomString(),
		Signature:     testutils.RandomString(),
	}
	credentialRedemptions := make([]cbr.CredentialRedemption, 0)
	credentialRedemptions = append(credentialRedemptions, credentialRedemption)

	credentials, err := json.Marshal(credentialRedemptions)
	suite.Require().NoError(err, "should have serialized credentials")

	query := `INSERT INTO claim_drain (credentials, wallet_id, total, transaction_id, erred, status, completed, updated_at) 
				VALUES ($1, $2, $3, $4, $5, $6, $7, now() - (interval '11 MINUTE')) RETURNING *;`

	var drainJob DrainJob
	err = pg.RawDB().Get(&drainJob, query, credentials, walletID, 1, transactionID, false, status, false)
	suite.Require().NoError(err, "should have inserted and returned claim drain row")

	return drainJob
}

func isCBRBypass(ctx context.Context) gomock.Matcher {
	return cbrBypass{ctx: ctx}
}

type cbrBypass struct {
	ctx context.Context
}

func (c cbrBypass) Matches(arg interface{}) bool {
	ctx := arg.(context.Context)
	return ctx.Value(appctx.SkipRedeemCredentialsCTXKey) == true
}

func (c cbrBypass) String() string {
	return "failed: cbr bypass is false"
}

func TestPostgresTestSuite(t *testing.T) {
	suite.Run(t, new(PostgresTestSuite))
}

func getClaimDrainEntry(pg *Postgres) *DrainJob {
	var dj = new(DrainJob)
	statement := `select * from claim_drain limit 1`
	_ = pg.Get(dj, statement)
	return dj
}

func getSuggestionDrainEntry(pg *Postgres) *SuggestionJob {
	var sj = new(SuggestionJob)
	statement := `select * from suggestion_drain limit 1`
	_ = pg.Get(sj, statement)
	return sj
}

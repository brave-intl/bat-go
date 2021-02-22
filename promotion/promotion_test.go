package promotion

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type PromotionTestSuite struct {
	suite.Suite
}

func (suite *PromotionTestSuite) SetupSuite() {
	// pg, _, err := NewPostgres()
	// suite.Require().NoError(err, "Failed to get postgres conn")

	// m, err := pg.NewMigrate()
	// suite.Require().NoError(err, "Failed to create migrate instance")

	// ver, dirty, _ := m.Version()
	// if dirty {
	// 	suite.Require().NoError(m.Force(int(ver)))
	// }
	// if ver > 0 {
	// 	suite.Require().NoError(m.Down(), "Failed to migrate down cleanly")
	// }

	// suite.Require().NoError(pg.Migrate(), "Failed to fully migrate")
}

func (suite *PromotionTestSuite) SetupTest() {
	suite.CleanDB()
}

func (suite *PromotionTestSuite) TearDownTest() {
	suite.CleanDB()
}

func (suite *PromotionTestSuite) CleanDB() {
	// tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions"}

	// pg, _, err := NewPostgres()
	// suite.Require().NoError(err, "Failed to get postgres conn")

	// for _, table := range tables {
	// 	_, err = pg.RawDB().Exec("delete from " + table)
	// 	suite.Require().NoError(err, "Failed to get clean table")
	// }
}

func TestPromotionTestSuite(t *testing.T) {
	suite.Run(t, new(PromotionTestSuite))
}

func (suite *PromotionTestSuite) TestPromotionExpired() {
	p := Promotion{
		ExpiresAt: time.Now(),
	}
	suite.Require().True(p.Expired())
	p.ExpiresAt = p.ExpiresAt.AddDate(0, 0, 1)
	suite.Require().False(p.Expired())
}

type Assertion struct {
	Claimable bool
	Promotion Promotion
}

func (suite *PromotionTestSuite) TestPromotionClaimable() {
	now := time.Now()
	// nowPlus := now.Add(time.Minute)
	monthsAgo3 := now.AddDate(0, -3, 0)
	scenarios := []Assertion{{
		Claimable: false,
		Promotion: Promotion{
			Active:        false, // fails because not active
			LegacyClaimed: false,
			CreatedAt:     monthsAgo3.Add(time.Minute),
			ExpiresAt:     now.Add(time.Minute),
		},
	}, {
		Claimable: true,
		Promotion: Promotion{
			Active:        true,
			LegacyClaimed: false,
			CreatedAt:     monthsAgo3.Add(time.Minute),
			ExpiresAt:     now.Add(time.Minute),
		},
	}, {
		Claimable: true,
		Promotion: Promotion{
			Active:        true, // fails because not active
			LegacyClaimed: true,
			CreatedAt:     monthsAgo3.Add(time.Minute),
			ExpiresAt:     now.Add(time.Minute),
		},
	}, {
		Claimable: false,
		Promotion: Promotion{
			Active:        true, // fails because not active
			LegacyClaimed: true,
			CreatedAt:     monthsAgo3,
			ExpiresAt:     now,
		},
	}, {
		Claimable: true,
		Promotion: Promotion{
			Active:        true, // fails because not active
			LegacyClaimed: false,
			CreatedAt:     monthsAgo3.Add(time.Minute),
			ExpiresAt:     now.Add(time.Minute),
		},
	}}
	for _, s := range scenarios {
		suite.Require().Equal(s.Claimable, s.Promotion.Claimable())
	}
}

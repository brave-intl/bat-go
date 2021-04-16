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
}

func (suite *PromotionTestSuite) SetupTest() {
}

func (suite *PromotionTestSuite) TearDownTest() {
}

func TestPromotionTestSuite(t *testing.T) {
	suite.Run(t, new(PromotionTestSuite))
}

func (suite *PromotionTestSuite) TestPromotionExpired() {
	p := Promotion{
		ExpiresAt: time.Now().UTC(),
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
			Active:        true,
			LegacyClaimed: true,
			CreatedAt:     monthsAgo3.Add(time.Minute),
			ExpiresAt:     now.Add(time.Minute),
		},
	}, {
		Claimable: false,
		Promotion: Promotion{
			Active:        true,
			LegacyClaimed: true,
			CreatedAt:     monthsAgo3,
			ExpiresAt:     now,
		},
	}, {
		Claimable: true,
		Promotion: Promotion{
			Active:        true,
			LegacyClaimed: false,
			CreatedAt:     monthsAgo3.Add(time.Minute),
			ExpiresAt:     now.Add(time.Minute),
		},
	}}
	for _, s := range scenarios {
		suite.Require().Equal(s.Claimable, s.Promotion.Claimable())
	}
}

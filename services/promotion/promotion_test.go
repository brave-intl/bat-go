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
		ExpiresAt: time.Now().Add(-1 * time.Second),
	}
	suite.Require().True(p.Expired())
	p.ExpiresAt = p.ExpiresAt.AddDate(0, 0, 1)
	suite.Require().False(p.Expired())
}

type Assertion struct {
	Claimable     bool
	LegacyClaimed bool
	Promotion     Promotion
}

func (suite *PromotionTestSuite) TestPromotionClaimable() {
	now := time.Now()
	monthsAgo3 := now.AddDate(0, -3, 0)
	scenarios := []Assertion{{
		Claimable:     true, // we no longer do Gone if the promotion active flag is false
		LegacyClaimed: false,
		Promotion: Promotion{
			Active:    false, // fails because not active
			CreatedAt: monthsAgo3.Add(time.Minute),
			ExpiresAt: now.Add(time.Minute),
		},
	}, {
		Claimable:     true,
		LegacyClaimed: false,
		Promotion: Promotion{
			Active:    true,
			CreatedAt: monthsAgo3.Add(time.Minute),
			ExpiresAt: now.Add(time.Minute),
		},
	}, {
		Claimable:     true,
		LegacyClaimed: true,
		Promotion: Promotion{
			Active:    true,
			CreatedAt: monthsAgo3.Add(time.Minute),
			ExpiresAt: now.Add(time.Minute),
		},
	}, {
		Claimable:     false,
		LegacyClaimed: true,
		Promotion: Promotion{
			Active:    true,
			CreatedAt: monthsAgo3,
			ExpiresAt: now,
		},
	}, {
		Claimable:     true,
		LegacyClaimed: false,
		Promotion: Promotion{
			Active:    true,
			CreatedAt: monthsAgo3.Add(time.Minute),
			ExpiresAt: now.Add(time.Minute),
		},
	}}
	for _, s := range scenarios {
		suite.Require().Equal(s.Claimable, s.Promotion.Claimable(s.LegacyClaimed))
	}
}

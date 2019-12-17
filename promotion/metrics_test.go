package promotion

import (
	"testing"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
)

type MetricsTestSuite struct {
	suite.Suite
}

func TestMetricsTestSuite(t *testing.T) {
	suite.Run(t, new(MetricsTestSuite))
}

func (suite *MetricsTestSuite) SetupSuite() {
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

	enableSuggestionJob = true
}

func (suite *MetricsTestSuite) SetupTest() {
	suite.clearDb()
}

func (suite *MetricsTestSuite) TearDownTest() {
	suite.clearDb()
}

func (suite *MetricsTestSuite) clearDb() {
	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions", "daily_unique_metrics"}

	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.DB.Exec("delete from " + table)
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func (suite *MetricsTestSuite) TestCountActiveWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	var ids []uuid.UUID
	for i := 0; i < 4; i++ {
		id := uuid.NewV4()
		err := pg.CountActiveWallet(id)
		suite.Require().NoError(err, "inserting should not fail")
		ids = append(ids, id)
	}

	for i := 0; i < len(ids); i++ {
		err := pg.CountActiveWallet(ids[i])
		suite.Require().NoError(err, "inserting should not fail")
	}

	var dailyUniques []DailyUniqueMetricCounts
	err = pg.DB.Select(&dailyUniques, `
	SELECT
		date,
		activity_type,
		hll_cardinality(wallets) as wallets
	FROM daily_unique_metrics`)
	suite.Assert().NoError(err)
	suite.Assert().Equal(1, len(dailyUniques), "only one activity was recorded")
	suite.Assert().Equal(4, dailyUniques[0].Wallets, "user has been seen")
}

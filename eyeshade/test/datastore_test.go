// +build eyeshade

package test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/eyeshade/datastore"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/eyeshade/must"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type DatastoreSuite struct {
	suite.Suite
	ctx  context.Context
	db   datastore.Datastore
	mock sqlmock.Sqlmock
}

func TestDatastoreSuite(t *testing.T) {
	suite.Run(t, new(DatastoreSuite))
	// if os.Getenv("EYESHADE_DB_URL") != "" {
	// 	suite.Run(t, new(DatastoreSuite))
	// }
}

func (suite *DatastoreSuite) SetupSuite() {
	ctx := context.Background()
	// setup mock DB we will inject into our pg
	db, err := datastore.NewDB(
		os.Getenv("EYESHADE_DB_URL"),
		true,
		"eyeshade",
	)
	suite.Require().NoError(err)
	suite.db = db
	suite.ctx = ctx
}

func ResetDB(ctx context.Context, db datastore.Datastore) error {
	tables := []string{
		"transactions",
		"votes",
		"surveyor_groups",
		"geo_referral_countries",
		"geo_referral_groups",
	}
	for _, table := range tables {
		statement := fmt.Sprintf(`delete from %s`, table)
		_, err := db.RawDB().
			ExecContext(ctx, statement)
		if err != nil {
			return err
		}
	}
	return db.SeedDB(ctx)
}

func (suite *DatastoreSuite) SetupTest() {
	suite.Require().NoError(
		ResetDB(suite.ctx, suite.db),
	)
}

func (suite *DatastoreSuite) TestInsertConvertableTransactions() {
	refs := must.CreateReferrals(3, models.OriginalRateID)
	groups, err := suite.db.GetActiveReferralGroups(suite.ctx)
	suite.Require().NoError(err)
	referrals, err := models.ReferralBackfillMany(
		&refs,
		groups,
		map[string]decimal.Decimal{"USD": decimal.NewFromFloat(2)},
	)
	suite.Require().NoError(err)
	referralConvertables := models.ReferralsToConvertableTransactions(
		*referrals...,
	)
	settlements := must.CreateSettlements(3, "contribution")
	settlements = models.SettlementBackfillMany(settlements)
	settlementConvertables := models.SettlementsToConvertableTransactions(
		settlements...,
	)
	convertables := append(referralConvertables, settlementConvertables...)
	suite.Require().NoError(
		suite.db.InsertConvertableTransactions(suite.ctx, convertables),
	)
	var p []models.Transaction
	statement := fmt.Sprintf(`
select %s from transactions`,
		strings.Join(models.TransactionColumns, ", "),
	)
	suite.Require().NoError(
		suite.db.RawDB().SelectContext(suite.ctx, &p, statement),
	)
}

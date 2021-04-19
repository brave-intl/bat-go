// +build eyeshade

package test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/eyeshade/datastore"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/eyeshade/must"
	"github.com/brave-intl/bat-go/utils/inputs"
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

func (suite *DatastoreSuite) TestInsertConvertableTransactions() {
	refs := must.CreateReferrals(3, models.OriginalRateID)
	groups, err := suite.db.GetReferralGroups(suite.ctx, *inputs.NewTime(time.RFC3339, time.Now()))
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
	settlementConvertables := models.SettlementsToConvertableTransactions(
		must.CreateSettlements(3, "contribution")...,
	)
	convertables := append(referralConvertables, settlementConvertables...)
	suite.Require().NoError(
		suite.db.InsertConvertableTransactions(suite.ctx, convertables),
	)
	var p []models.Transaction
	suite.Require().NoError(
		suite.db.RawDB().SelectContext(suite.ctx, &p, `select * from transactions`),
	)
	j, _ := json.Marshal(p)
	fmt.Println(string(j))
}

//go:build integration

package promotion

import (
	"context"
	"testing"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/inputs"
	"github.com/brave-intl/bat-go/libs/test"
	"github.com/brave-intl/bat-go/services/wallet"

	// re-using viper bind-env for wallet env variables
	_ "github.com/brave-intl/bat-go/services/wallet/cmd"
	"github.com/stretchr/testify/suite"
)

type ServiceTestSuite struct {
	suite.Suite
}

func (suite *ServiceTestSuite) SetupSuite() {
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

func (suite *ServiceTestSuite) SetupTest() {
	suite.CleanDB()
}

func (suite *ServiceTestSuite) TearDownTest() {
	suite.CleanDB()
}

func (suite *ServiceTestSuite) CleanDB() {
	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions"}

	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.RawDB().Exec("delete from " + table)
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func TestServiceTestSuite(t *testing.T) {
	suite.Run(t, new(ServiceTestSuite))
}

func (suite *ServiceTestSuite) createService() (*Service, context.Context) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, appctx.ParametersMergeBucketCTXKey, test.RandomString())
	ctx = context.WithValue(ctx, appctx.DisabledWalletGeoCountriesCTXKey, test.RandomString())

	ctx, walletService := wallet.SetupService(ctx)
	promotionDB, promotionRODB, err := NewPostgres()
	suite.Require().NoError(err, "unable connect to promotion db")
	s, err := InitService(
		ctx,
		promotionDB,
		promotionRODB,
		walletService,
	)
	suite.Require().NoError(err)

	return s, ctx
}

func (suite *ServiceTestSuite) TestGetAvailablePromotions() {
	var nilPromotions *[]Promotion
	noPromotions := []Promotion{}
	service, ctx := suite.createService()

	walletID := new(inputs.ID)
	id := walletID.UUID()

	promotions, err := service.GetAvailablePromotions(ctx, id, "", true)
	suite.Require().NoError(err)
	suite.Require().Equal(&noPromotions, promotions)

	err = inputs.DecodeAndValidateString(
		ctx,
		walletID,
		"00000000-0000-0000-0000-000000000000",
	)
	suite.Require().NoError(err)

	id = walletID.UUID()
	promotions, err = service.GetAvailablePromotions(ctx, id, "", true)
	suite.Require().NoError(err)
	suite.Require().Equal(nilPromotions, promotions)
}

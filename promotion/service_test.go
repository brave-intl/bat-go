// +build integration

package promotion

import (
	"context"
	"testing"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/go-chi/chi"
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
	r := chi.NewRouter()
	r, ctx, walletService := cmd.SetupWalletService(ctx, r)
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
	service, ctx := suite.createService()
	noPromotions := []Promotion{}

	walletID := new(inputs.ID)
	id := walletID.UUID()

	promotions, err := service.GetAvailablePromotions(ctx, id, "", true)
	suite.Require().NoError(err)
	suite.Require().Equal(&noPromotions, promotions)
}

// +build integration

package wallet

import (
	"testing"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
)

type WalletPostgresTestSuite struct {
	suite.Suite
}

func TestWalletPostgresTestSuite(t *testing.T) {
	suite.Run(t, new(WalletPostgresTestSuite))
}

func (suite *WalletPostgresTestSuite) SetupSuite() {
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
}

func (suite *WalletPostgresTestSuite) SetupTest() {
	suite.CleanDB()
}

func (suite *WalletPostgresTestSuite) TearDownTest() {
	suite.CleanDB()
}

func (suite *WalletPostgresTestSuite) CleanDB() {
	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions"}

	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.DB.Exec("delete from " + table)
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func (suite *WalletPostgresTestSuite) TestInsertWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	wallet := &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(wallet), "Save wallet should succeed")
}

func (suite *WalletPostgresTestSuite) TestGetWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	id := uuid.NewV4()

	walletc := altcurrency.BAT
	origWallet := &walletutils.Info{ID: id.String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey, AltCurrency: &walletc}
	suite.Require().NoError(pg.InsertWallet(origWallet), "Save wallet should succeed")

	wallet, err := pg.GetWallet(id)
	suite.Require().NoError(err, "Get wallet should succeed")
	suite.Require().Equal(origWallet, wallet)
}

// +build integration

package wallet

import (
	"context"
	"database/sql"
	"errors"
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

func (suite *WalletPostgresTestSuite) SetupTest() {
	suite.CleanDB()
}

func (suite *WalletPostgresTestSuite) TearDownTest() {
	suite.CleanDB()
}

func (suite *WalletPostgresTestSuite) CleanDB() {
	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions"}

	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.RawDB().Exec("delete from " + table)
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func (suite *WalletPostgresTestSuite) TestInsertWallet() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	wallet := &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.InsertWallet(context.Background(), wallet), "Save wallet should succeed")
}

func (suite *WalletPostgresTestSuite) TestUpsertWallet() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	wallet := &walletutils.Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.UpsertWallet(context.Background(), wallet), "Save wallet should succeed")
}

func (suite *WalletPostgresTestSuite) TestGetWallet() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	id := uuid.NewV4()

	tmp := altcurrency.BAT
	origWallet := &walletutils.Info{ID: id.String(), Provider: "uphold", AltCurrency: &tmp, ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.UpsertWallet(context.Background(), origWallet), "Save wallet should succeed")

	wallet, err := pg.GetWallet(context.Background(), id)
	suite.Require().NoError(err, "Get wallet should succeed")
	suite.Assert().Equal(origWallet, wallet)
}

func (suite *WalletPostgresTestSuite) TestCustodianLink() {

	ctx := context.Background()

	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	// setup a wallet
	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	id := uuid.NewV4()
	depositDest := uuid.NewV4()
	linkingID := uuid.NewV4()

	tmp := altcurrency.BAT
	origWallet := &walletutils.Info{ID: id.String(), Provider: "uphold", AltCurrency: &tmp, ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Require().NoError(pg.UpsertWallet(context.Background(), origWallet), "Save wallet should succeed")

	// perform a connect custodial wallet
	suite.Require().NoError(
		pg.ConnectCustodialWallet(ctx, CustodianLink{
			ID:                 nil, // to create a new custodian link
			WalletID:           &id,
			Custodian:          "gemini",
			DepositDestination: depositDest.String(),
			LinkingID:          &linkingID,
		}),
		"Connect Custodial Wallet wallet should succeed")

	// get the wallet and check that the custodian link entry id is right
	// get the custodian link entry and validate that the data is correct
	cl, err := pg.GetCustodianLinkByWalletID(ctx, id)
	suite.Require().NoError(err, "should have no error getting custodian link")
	suite.Require().True(cl.LinkingID.String() == linkingID.String(), "linking id is not right")
	suite.Require().True(cl.WalletID.String() == id.String(), "wallet id is not right")
	suite.Require().True(cl.Custodian == "gemini", "custodian is not right")

	clID := cl.ID
	// check the link count is 1 for this wallet
	used, max, err := pg.GetCustodianLinkCount(ctx, linkingID)
	suite.Require().NoError(err, "should have no error getting custodian link count")
	suite.Require().True(used == 1, "linking count is not right")
	suite.Require().True(max == getEnvMaxCards(), "linking count is not right")

	// perform a disconnect custodial wallet
	suite.Require().NoError(
		pg.DisconnectCustodialWallet(ctx, id),
		"disconnect Custodial Wallet wallet should succeed")

	// should return sql not found error after a disconnect
	cl, err = pg.GetCustodianLinkByWalletID(ctx, id)
	suite.Require().True(errors.Is(err, sql.ErrNoRows), "should be no rows found error")

	// make sure the immutable record still exists
	cl, err = pg.GetCustodianLinkByID(ctx, *clID)
	suite.Require().NoError(err, "should be no errors, should still exist")
}

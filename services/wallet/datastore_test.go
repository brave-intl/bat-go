//go:build integration

package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/clients/reputation"
	mockreputation "github.com/brave-intl/bat-go/libs/clients/reputation/mock"
	"github.com/golang/mock/gomock"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	appctx "github.com/brave-intl/bat-go/libs/context"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
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

	ctx := context.WithValue(context.Background(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D")

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
		pg.ConnectCustodialWallet(ctx, &CustodianLink{
			WalletID:  &id,
			Custodian: "gemini",
			LinkingID: &linkingID,
		}, depositDest.String()),
		"connect custodial wallet should succeed")

	// get the wallet and check that the custodian link entry id is right
	// get the custodian link entry and validate that the data is correct
	cl, err := pg.GetCustodianLinkByWalletID(ctx, id)
	suite.Require().NoError(err, "should have no error getting custodian link")
	suite.Require().True(cl.LinkingID.String() == linkingID.String(), "linking id is not right")
	suite.Require().True(cl.WalletID.String() == id.String(), "wallet id is not right")
	suite.Require().True(cl.Custodian == "gemini", "custodian is not right")

	// check the link count is 1 for this wallet
	used, max, err := pg.GetCustodianLinkCount(ctx, linkingID, "gemini")
	suite.Require().NoError(err, "should have no error getting custodian link count")

	// disconnect the wallet
	suite.Require().NoError(
		pg.DisconnectCustodialWallet(ctx, id),
		"connect custodial wallet should succeed")

	// connect a custodial wallet to make sure not more than one linking is added for same cust/wallet
	suite.Require().NoError(
		pg.ConnectCustodialWallet(ctx, &CustodianLink{
			WalletID:  &id,
			Custodian: "gemini",
			LinkingID: &linkingID,
		}, depositDest.String()),
		"connect custodial wallet should succeed")

	// only one slot should be taken
	suite.Require().True(used == 1, "linking count is not right")
	suite.Require().True(max == getEnvMaxCards("gemini"), "linking count is not right")

	// perform a disconnect custodial wallet
	suite.Require().NoError(
		pg.DisconnectCustodialWallet(ctx, id),
		"disconnect custodial wallet should succeed")

	// should return sql not found error after a disconnect
	cl, err = pg.GetCustodianLinkByWalletID(ctx, id)
	suite.Require().True(errors.Is(err, sql.ErrNoRows), "should be no rows found error")
}

func (suite *WalletPostgresTestSuite) TestConnectCustodialWallet_Rollback() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	ctx := context.Background()

	walletID := uuid.NewV4()
	linkingID := uuid.NewV4()
	depositDest := uuid.NewV4().String()

	err = pg.ConnectCustodialWallet(ctx, &CustodianLink{
		WalletID:  &walletID,
		Custodian: "uphold",
		LinkingID: &linkingID,
	}, depositDest)

	suite.Require().True(err != nil, "should have returned error")

	count, _, err := pg.GetCustodianLinkCount(ctx, linkingID, "")

	suite.Require().NoError(err)
	suite.Require().True(count == 0, "should have performed rollback on connect custodial wallet")
}

func (suite *WalletPostgresTestSuite) TestLinkWallet_Concurrent_InsertUpdate() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	// Mock reputation client calls and enable verified wallet
	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	VerifiedWalletEnable = true
	reputationClient := mockreputation.NewMockClient(mockCtrl)

	ctx = context.WithValue(ctx, appctx.ReputationClientCTXKey, reputationClient)
	reputationClient.EXPECT().
		UpdateReputationSummary(gomock.Any(),
			gomock.Any(), true).
		Return(nil).
		AnyTimes()

	reputationClient.EXPECT().IsLinkingReputable(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(true, []int{reputation.CohortTooYoung, reputation.CohortGeoResetDifferent}, nil).
		AnyTimes()

	// Create the maximum wallets - 1 with the same linkingID
	userDepositDestination := uuid.NewV4().String()
	providerLinkingID := uuid.NewV4()

	ctx = context.WithValue(ctx, appctx.NoUnlinkPriorToDurationCTXKey, "-P1D")

	used, max, err := pg.GetCustodianLinkCount(ctx, providerLinkingID, "")
	suite.Require().NoError(err)

	for i := 0; i < max-1; i++ {
		altCurrency := altcurrency.BAT
		walletInfo := &walletutils.Info{
			ID:               uuid.NewV4().String(),
			Provider:         "uphold",
			ProviderID:       uuid.NewV4().String(),
			AltCurrency:      &altCurrency,
			PublicKey:        "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu1jMwryY=",
			AnonymousAddress: nil,
		}

		err := pg.UpsertWallet(ctx, walletInfo)
		suite.Require().NoError(err, "save wallet should succeed")

		err = pg.LinkWallet(ctx, walletInfo.ID, userDepositDestination, providerLinkingID,
			walletInfo.AnonymousAddress, "uphold", "")
		suite.Require().NoError(err, "link wallet should succeed")
	}

	// Create a final wallet to reach maximum and make two concurrent link calls
	altCurrency := altcurrency.BAT
	walletInfo := &walletutils.Info{
		ID:               uuid.NewV4().String(),
		Provider:         "uphold",
		ProviderID:       uuid.NewV4().String(),
		AltCurrency:      &altCurrency,
		PublicKey:        "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu1jMwryY=",
		AnonymousAddress: nil,
	}
	err = pg.UpsertWallet(ctx, walletInfo)
	suite.Require().NoError(err)

	// Make two concurrent link calls
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			err = pg.LinkWallet(ctx, walletInfo.ID, userDepositDestination, providerLinkingID,
				walletInfo.AnonymousAddress, walletInfo.Provider, "")
		}()
	}
	wg.Wait()

	// Assert we have not linked more that the maximum allowed
	used, max, err = pg.GetCustodianLinkCount(ctx, providerLinkingID, "")
	suite.Require().NoError(err)
	suite.Require().True(used == max, fmt.Sprintf("used %d should not exceed max %d", used, max))
}

func (suite *WalletPostgresTestSuite) TestLinkWallet_Concurrent_MaxLinkCount() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	VerifiedWalletEnable = true
	reputationClient := mockreputation.NewMockClient(mockCtrl)

	ctx = context.WithValue(ctx, appctx.ReputationClientCTXKey, reputationClient)
	reputationClient.EXPECT().
		UpdateReputationSummary(gomock.Any(),
			gomock.Any(), true).
		Return(nil).
		AnyTimes()

	reputationClient.EXPECT().IsLinkingReputable(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(true, []int{reputation.CohortTooYoung,
			reputation.CohortGeoResetDifferent}, nil).
		AnyTimes()

	ctx = context.WithValue(ctx, appctx.NoUnlinkPriorToDurationCTXKey, "-P1D")

	wallets := make([]*walletutils.Info, 10, 10)

	for i := 0; i < len(wallets); i++ {
		altCurrency := altcurrency.BAT
		walletInfo := &walletutils.Info{
			ID:          uuid.NewV4().String(),
			Provider:    "uphold",
			ProviderID:  uuid.NewV4().String(),
			AltCurrency: &altCurrency,
			PublicKey:   "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu1jMwryY=",
		}
		wallets[i] = walletInfo
		err := pg.UpsertWallet(ctx, walletInfo)
		suite.Require().NoError(err, "save wallet should succeed")
	}

	var wg sync.WaitGroup
	wg.Add(len(wallets))

	userDepositDestination := uuid.NewV4().String()
	providerLinkingID := uuid.NewV4()

	for i := 0; i < len(wallets); i++ {
		go func(index int) {
			defer wg.Done()
			err = pg.LinkWallet(ctx, wallets[index].ID, userDepositDestination, providerLinkingID,
				wallets[index].AnonymousAddress, wallets[index].Provider, "")
		}(i)
	}

	wg.Wait()

	used, max, err := pg.GetCustodianLinkCount(ctx, providerLinkingID, "")

	suite.Require().NoError(err, "should have no error getting custodian link count")
	suite.Require().True(used == max, fmt.Sprintf("used %d should not exceed max %d", used, max))
}

func (suite *WalletPostgresTestSuite) TestWaitAndLock() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	actual := make(chan int, 2)

	f := func(ctx context.Context, waitSeconds int, process int, lockID uuid.UUID) {
		tx, err := pg.RawDB().Beginx()
		suite.Require().NoError(err)

		err = waitAndLockTx(ctx, tx, lockID)
		suite.Require().NoError(err)

		_, err = tx.ExecContext(ctx, "SELECT 1, pg_sleep($1)", waitSeconds)
		suite.Require().NoError(err)

		actual <- process

		err = tx.Commit()
		suite.Require().NoError(err)
	}

	lockID := uuid.NewV4()

	go f(context.Background(), 2, 1, lockID)
	time.Sleep(500 * time.Millisecond)
	go f(context.Background(), 0, 2, lockID)

	suite.Require().True(<-actual == 1)
	suite.Require().True(<-actual == 2)

	row := pg.RawDB().QueryRow("SELECT COUNT(*) FROM pg_locks pl WHERE pl.objid = hashtext($1)", lockID)

	var lockCount int
	err = row.Scan(&lockCount)
	suite.Require().True(lockCount == 0, fmt.Sprintf("should have released all locks but found %d", lockCount))
}

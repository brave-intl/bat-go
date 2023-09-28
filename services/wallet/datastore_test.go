//go:build integration

package wallet

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/libs/backoff/retrypolicy"
	mock_reputation "github.com/brave-intl/bat-go/libs/clients/reputation/mock"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/datastore"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/services/wallet/model"
	"github.com/golang/mock/gomock"
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
	suite.Require().True(errors.Is(err, model.ErrNoWalletCustodian), "should be no rows found error")
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
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	for i := 0; i < 1; i++ {

		// seed 3 wallets with same linkingID
		userDepositDestination, providerLinkingID := suite.seedWallet(pg)

		// concurrently link new wallet with same linkingID
		altCurrency := altcurrency.BAT
		walletInfo := &walletutils.Info{
			ID:          uuid.NewV4().String(),
			Provider:    "uphold",
			ProviderID:  uuid.NewV4().String(),
			AltCurrency: &altCurrency,
			PublicKey:   "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu1jMwryY=",
		}

		err = pg.UpsertWallet(context.WithValue(context.Background(),
			appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"), walletInfo)
		suite.Require().NoError(err, "save wallet should succeed")

		runs := 2
		var wg sync.WaitGroup
		wg.Add(runs)

		for i := 0; i < runs; i++ {
			go func() {
				defer wg.Done()
				err = pg.LinkWallet(context.WithValue(context.Background(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"),
					walletInfo.ID, userDepositDestination, providerLinkingID, walletInfo.AnonymousAddress, walletInfo.Provider, "")
			}()
		}

		wg.Wait()

		used, max, err := pg.GetCustodianLinkCount(context.WithValue(context.Background(),
			appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"), providerLinkingID, "")

		suite.Require().NoError(err, "should have no error getting custodian link count")
		suite.Require().True(used == max, fmt.Sprintf("used %d should not exceed max %d", used, max))
	}
}

func (suite *WalletPostgresTestSuite) seedWallet(pg Datastore) (string, uuid.UUID) {
	userDepositDestination := uuid.NewV4().String()
	providerLinkingID := uuid.NewV4()

	walletCount := 3
	for i := 0; i < walletCount; i++ {
		altCurrency := altcurrency.BAT
		walletInfo := &walletutils.Info{
			ID:               uuid.NewV4().String(),
			Provider:         "uphold",
			ProviderID:       uuid.NewV4().String(),
			AltCurrency:      &altCurrency,
			PublicKey:        "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu1jMwryY=",
			AnonymousAddress: nil,
		}

		err := pg.UpsertWallet(context.WithValue(context.Background(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"), walletInfo)
		suite.Require().NoError(err, "save wallet should succeed")

		err = pg.LinkWallet(context.WithValue(context.Background(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"),
			walletInfo.ID, userDepositDestination, providerLinkingID, walletInfo.AnonymousAddress, "uphold", "")
		suite.Require().NoError(err, "link wallet should succeed")
	}

	used, _, err := pg.GetCustodianLinkCount(context.WithValue(context.Background(),
		appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"), providerLinkingID, "")

	suite.Require().NoError(err, "should have no error getting custodian link count")
	suite.Require().True(used == walletCount, fmt.Sprintf("used %d", used))

	return userDepositDestination, providerLinkingID
}

func (suite *WalletPostgresTestSuite) TestLinkWallet_Concurrent_MaxLinkCount() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

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
		err := pg.UpsertWallet(context.WithValue(context.Background(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"), walletInfo)
		suite.Require().NoError(err, "save wallet should succeed")
	}

	var wg sync.WaitGroup
	wg.Add(len(wallets))

	userDepositDestination := uuid.NewV4().String()
	providerLinkingID := uuid.NewV4()

	for i := 0; i < len(wallets); i++ {
		go func(index int) {
			defer wg.Done()
			err = pg.LinkWallet(context.WithValue(context.Background(), appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"),
				wallets[index].ID, userDepositDestination, providerLinkingID, wallets[index].AnonymousAddress, wallets[index].Provider, "")
		}(i)
	}

	wg.Wait()

	used, max, err := pg.GetCustodianLinkCount(context.WithValue(context.Background(),
		appctx.NoUnlinkPriorToDurationCTXKey, "-P1D"), providerLinkingID, "")

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

func (suite *WalletPostgresTestSuite) TestSendVerifiedWalletOutbox() {
	ctx := context.Background()

	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	repClient := mock_reputation.NewMockClient(ctrl)

	_, tx, _, commit, err := datastore.GetTx(ctx, pg)
	suite.Require().NoError(err)

	var paymentIDs []uuid.UUID
	for i := 0; i < 5; i++ {

		// Insert the outbox request.
		paymentIDs = append(paymentIDs, uuid.NewV4())
		err = pg.InsertVerifiedWalletOutboxTx(ctx, tx, paymentIDs[i], true)
		suite.Require().NoError(err)

		// Mock the reputation call, the requests are ordered by created at so should be
		// processed in the order of insertion.
		repClient.EXPECT().
			UpdateReputationSummary(ctx, paymentIDs[i].String(), true).
			Return(nil)
	}

	err = commit()
	suite.Require().NoError(err)

	// Send the requests
	retryPolicy = retrypolicy.NoRetry
	for i := 0; i < 5; i++ {
		_, err = pg.SendVerifiedWalletOutbox(ctx, repClient, backoff.Retry)
		suite.Require().NoError(err)
	}

	// Assert all request have been processed, the table should be empty.
	var count int
	err = pg.RawDB().GetContext(ctx, &count, `select count(*) from verified_wallet_outbox`)
	suite.Require().NoError(err)
	suite.Assert().Equal(0, count)
}

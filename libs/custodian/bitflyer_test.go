//go:build custodianintegration
// +build custodianintegration

package custodian_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/custodian"
	logutils "github.com/brave-intl/bat-go/libs/logging"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type BitflyerCustodianTestSuite struct {
	suite.Suite
	custodian custodian.Custodian
	ctx       context.Context
}

func TestBitflyerCustodianTestSuite(t *testing.T) {
	suite.Run(t, new(BitflyerCustodianTestSuite))
}

func (suite *BitflyerCustodianTestSuite) SetupTest() {
	// setup the context
	suite.ctx = context.Background()

	// setup debug for client
	suite.ctx = context.WithValue(suite.ctx, appctx.DebugLoggingCTXKey, true)
	// setup debug log level
	suite.ctx = context.WithValue(suite.ctx, appctx.LogLevelCTXKey, "debug")

	// setup a logger and put on context
	suite.ctx, _ = logutils.SetupLogger(suite.ctx)

	for _, key := range []appctx.CTXKey{
		appctx.BitFlyerJWTKeyCTXKey,
		appctx.BitflyerExtraClientSecretCTXKey,
		appctx.BitflyerClientSecretCTXKey,
		appctx.BitflyerClientIDCTXKey,
		appctx.BitflyerServerURLCTXKey,
		appctx.BitflyerProxyURLCTXKey,
		appctx.BitflyerTokenCTXKey,
		appctx.BitflyerSourceFromCTXKey} {
		// setup keys
		suite.ctx = context.WithValue(suite.ctx, key, os.Getenv(strings.ToUpper(string(key))))
	}

	var err error
	// setup custodian, all configs default to whats in context
	suite.custodian, err = custodian.New(suite.ctx, custodian.Config{Provider: "bitflyer"})
	suite.Require().NoError(err, "Must be able to correctly initialize the client")
}

func (suite *BitflyerCustodianTestSuite) TestSubmitAndGetTransactions() {
	var (
		// source is settlement wallet and in bf case there is no source
		source = uuid.New()

		// dest is destination wallet
		dest1 = uuid.MustParse(os.Getenv("BITFLYER_TEST_DESTINATION_ID"))
		dest2 = uuid.MustParse(os.Getenv("BITFLYER_TEST_DESTINATION_ID"))
	)

	// txs
	ik1 := uuid.New()
	ik2 := uuid.New()

	oneBAT, err := decimal.NewFromString("1")
	twoBAT, err := decimal.NewFromString("2")

	tx1, err := custodian.NewTransaction(suite.ctx, &ik1, &dest1, &source, altcurrency.BAT, oneBAT)
	suite.Require().NoError(err, "should be able to create transactions")
	tx2, err := custodian.NewTransaction(suite.ctx, &ik2, &dest2, &source, altcurrency.BAT, twoBAT)
	suite.Require().NoError(err, "should be able to create transactions")

	txs := []custodian.Transaction{tx1, tx2}

	err = suite.custodian.SubmitTransactions(suite.ctx, txs...)
	suite.Require().NoError(err, "should be able to submit transactions")

	statusMap, err := suite.custodian.GetTransactionsStatus(suite.ctx, txs...)
	suite.Require().NoError(err, "should be able to get transactions status")

	suite.Require().True(len(statusMap) == 1, "status map should have collapsed transaction statuses")
}

func (suite *BitflyerCustodianTestSuite) TestSubmitAndGetTransactionsOneOff() {
	var (
		// source is settlement wallet and in bf case there is no source
		source = uuid.New()

		// dest is destination wallet
		dest1 = uuid.MustParse("31987fe9-51c2-48d4-8106-76ee5f613da7")
		dest2 = uuid.MustParse("31987fe9-51c2-48d4-8106-76ee5f613da8")
	)

	// txs
	ik1 := uuid.New()
	ik2 := uuid.New()

	fiveBAT, err := decimal.NewFromString("5")

	tx1, err := custodian.NewTransaction(suite.ctx, &ik1, &dest1, &source, altcurrency.BAT, fiveBAT)
	suite.Require().NoError(err, "should be able to create transactions")

	tx2, err := custodian.NewTransaction(suite.ctx, &ik2, &dest2, &source, altcurrency.BAT, fiveBAT)
	suite.Require().NoError(err, "should be able to create transactions")

	txs := []custodian.Transaction{tx1, tx2}

	err = suite.custodian.SubmitTransactions(suite.ctx, txs...)
	suite.Require().NoError(err, "should be able to submit transactions")

	statusMap, err := suite.custodian.GetTransactionsStatus(suite.ctx, txs...)
	suite.Require().NoError(err, "should be able to get transactions status")

	fmt.Println(statusMap)
	suite.Require().True(len(statusMap) == 1, "status map should have collapsed transaction statuses")
}

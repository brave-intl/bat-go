// +build integration,vpn

package bitflyer

import (
	"context"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/cryptography"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type BitflyerTestSuite struct {
	suite.Suite
	secret cryptography.HMACKey
}

func TestBitflyerTestSuite(t *testing.T) {
	suite.Run(t, new(BitflyerTestSuite))
}

func (suite *BitflyerTestSuite) SetupTest() {
}

func (suite *BitflyerTestSuite) TestBulkPay() {
	ctx := context.Background()
	client, err := New()
	suite.Require().NoError(err, "Must be able to correctly initialize the client")
	one := decimal.NewFromFloat(1)

	quote, err := client.FetchQuote(ctx, "BAT_JPY")
	suite.Require().NoError(err, "fetching a quote does not fail")

	dryRun := true
	tx := settlement.Transaction{
		SettlementID: uuid.NewV4().String(),
		Destination:  os.Getenv("BITFLYER_TEST_DESTINATION_ID"),
		Channel:      "brave.com",
		Probi:        altcurrency.BAT.ToProbi(one),
		Amount:       one,
	}
	sourceFrom := os.Getenv("BITFLYER_SOURCE_FROM")
	if sourceFrom == "" {
		sourceFrom = "self"
	}
	txs := []settlement.Transaction{tx}
	withdrawals, err := NewWithdrawsFromTxs(sourceFrom, &txs)
	suite.Require().NoError(err)
	bulkTransferRequest := NewWithdrawToDepositIDBulkPayload(
		dryRun,
		quote.PriceToken,
		withdrawals,
	)

	bulkPayoutResponse, err := client.UploadBulkPayout(ctx, *bulkTransferRequest)
	expectedPayoutResults := WithdrawToDepositIDBulkResponse{
		Withdrawals: []WithdrawToDepositIDResponse{
			{
				TransferID:   GenerateTransferID(&tx),
				Amount:       one,
				Message:      "",
				Status:       "SUCCESS",
				CurrencyCode: "BAT",
			},
		},
	}
	suite.Require().NoError(err, "should not error during bulk payout uploading")
	suite.Require().Equal(&expectedPayoutResults, bulkPayoutResponse, "the response should be predictable")
}

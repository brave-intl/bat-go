//go:build integration && vpn
// +build integration,vpn

package bitflyer

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients"
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

	quote, err := client.FetchQuote(ctx, "BAT_JPY", true)
	suite.Require().NoError(err, "fetching a quote does not fail")

	dryRun := &DryRunOption{}
	tx := settlement.Transaction{
		SettlementID: uuid.NewV4().String(),
		Destination:  os.Getenv("BITFLYER_TEST_DESTINATION_ID"),
		Channel:      "brave.com",
		Probi:        altcurrency.BAT.ToProbi(one),
		Amount:       one,
	}
	sourceFrom := os.Getenv("BITFLYER_SOURCE_FROM")
	if sourceFrom == "" {
		sourceFrom = "tipping"
	}
	txs := []settlement.Transaction{tx}
	withdrawals, err := NewWithdrawsFromTxs(sourceFrom, txs)
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
				TransferID:   tx.TransferID(),
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

type nopCloser struct {
	*bytes.Buffer
}

func (nc nopCloser) Close() error {
	return nil
}

func (suite *BitflyerTestSuite) TestHandleBitflyerError() {
	buf := bytes.NewBufferString(`
{
	"status": -1,
	"label": "JsonError.TOKEN_ERROR",
	"message": "認証に失敗しました。",
	"errors": [
		"242503"
	]
}
	`)
	body := nopCloser{buf}
	resp := http.Response{
		StatusCode: http.StatusUnauthorized,
		Body:       body,
	}

	err := handleBitflyerError(errors.New("failed"), nil, &resp)
	var bfError *clients.BitflyerError
	if errors.As(err, &bfError) {
		suite.Require().Equal(bfError.HTTPStatusCode, http.StatusUnauthorized, "status should match")
		suite.Require().Equal(bfError.Status, -1, "status should match")
	} else {
		suite.Require().True(false, "should not be another type of error")
	}

}

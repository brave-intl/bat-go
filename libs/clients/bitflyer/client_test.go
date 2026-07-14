//go:build integration && vpn

package bitflyer

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/custodian"
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
	tx := custodian.Transaction{
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
	txs := []custodian.Transaction{tx}
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

func TestHandleBitflyerError(t *testing.T) {
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

	err := handleBitflyerError(context.Background(), errors.New("failed"), &resp)
	var bfError *clients.BitflyerError
	if errors.As(err, &bfError) {
		assert.Equal(t, bfError.HTTPStatusCode, http.StatusUnauthorized, "status should match")
		assert.Equal(t, bfError.Status, -1, "status should match")
	} else {
		assert.Fail(t, "should not be another type of error")
	}
}

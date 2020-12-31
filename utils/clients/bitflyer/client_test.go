// +build integration,vpn

package bitflyer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/settlement"
	bitflyersettlement "github.com/brave-intl/bat-go/settlement/bitflyer"
	"github.com/brave-intl/bat-go/utils/cryptography"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type BitflyerTestSuite struct {
	suite.Suite
	secret cryptography.HMACKey
	apikey string
}

func TestBitflyerTestSuite(t *testing.T) {
	suite.Run(t, new(BitflyerTestSuite))
}

func (suite *BitflyerTestSuite) SetupTest() {
	secret := os.Getenv("BITFLYER_CLIENT_SECRET")
	apikey := os.Getenv("BITFLYER_CLIENT_KEY")
	if secret != "" {
		suite.secret = cryptography.NewHMACHasher([]byte(secret))
		suite.apikey = apikey
	}
}

func (suite *BitflyerTestSuite) TestBulkPay() {
	ctx := context.Background()
	client, err := New()
	suite.Require().NoError(err, "Must be able to correctly initialize the client")
	five := decimal.NewFromFloat(5)

	quote, err := client.FetchQuote(ctx, "BAT_JPY")
	suite.Require().NoError(err, "fetching a quote does not fail")

	dryRun := true
	tx := settlement.Transaction{
		SettlementID: uuid.NewV4().String(),
		Destination:  os.Getenv("BITFLYER_TEST_DESTINATION_ID"),
		Channel:      "brave.com",
	}
	sourceFrom := os.Getenv("BITFLYER_TEST_SOURCE")
	txs := []settlement.Transaction{tx}
	withdrawals := NewWithdrawRequestFromTxs(sourceFrom, &txs)
	bulkTransferRequest := NewWithdrawToDepositIDBulkRequest(
		&dryRun,
		quote.PriceToken,
		withdrawals,
	)

	bulkPayoutResponse, err := client.UploadBulkPayout(ctx, suite.apikey, suite.secret, bulkTransferRequest)
	pendingStatus := "Pending"
	expectedPayoutResult := PayoutResult{
		Result:      "OK",
		TxRef:       GenerateTransferID(&tx),
		Amount:      &five,
		Currency:    &BAT,
		Destination: &tx.Destination,
		Status:      &pendingStatus,
	}
	expectedPayoutResults := []PayoutResult{expectedPayoutResult}
	suite.Require().NoError(err, "should not error during bulk payout uploading")
	suite.Require().Equal(&expectedPayoutResults, bulkPayoutResponse, "the response should be predictable")

	status, err := client.CheckPayoutStatus(
		ctx,
		suite.apikey,
		os.Getenv("BITFLYER_CLIENT_ID"),
		GenerateTransferID(&tx),
	)
	suite.Require().NoError(err, "should not error during bulk payout uploading")
	suite.Require().Equal(&expectedPayoutResult, status, "checking the single response should be predictable")
}

func findBalanceByCurrency(balances *[]Balance, currency string) Balance {
	for _, balance := range *balances {
		if balance.Currency == currency {
			return balance
		}
	}
	return Balance{}
}

func findAccountByClass(accounts *[]Account, typ string) Account {
	for _, account := range *accounts {
		if account.Class == typ {
			return account
		}
	}
	return Account{}
}

func (suite *BitflyerTestSuite) preparePrivateRequest(payload interface{}) string {
	payloadSerialized, err := json.Marshal(payload)
	suite.Require().NoError(err, "payload must be able to be serialized")

	return base64.StdEncoding.EncodeToString(payloadSerialized)
}

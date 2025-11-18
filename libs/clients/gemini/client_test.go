//go:build integration && vpn

package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/custodian"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type GeminiTestSuite struct {
	suite.Suite
	secret cryptography.HMACKey
	apikey string
}

func TestGeminiTestSuite(t *testing.T) {
	suite.Run(t, new(GeminiTestSuite))
}

func (suite *GeminiTestSuite) SetupTest() {
	secret := os.Getenv("GEMINI_CLIENT_SECRET")
	apikey := os.Getenv("GEMINI_CLIENT_KEY")
	if secret != "" {
		suite.secret = cryptography.NewHMACHasher([]byte(secret))
		suite.apikey = apikey
	}
}

func (suite *GeminiTestSuite) TestBulkPay() {
	ctx := context.Background()
	client, err := New()
	suite.Require().NoError(err, "Must be able to correctly initialize the client")
	five := decimal.NewFromFloat(5)

	var accountKey *string
	if strings.Split(suite.apikey, "-")[0] == "master" {
		accountListRequest := suite.preparePrivateRequest(NewAccountListPayload())
		accounts, err := client.FetchAccountList(ctx, suite.apikey, suite.secret, accountListRequest)
		suite.Require().NoError(err, "should not error during account list fetching")
		primary := "primary"
		account := findAccountByClass(accounts, primary)
		suite.Require().Equal(primary, account.Class, "should have a primary account")
		accountKey = &primary
	}

	balancesRequest := suite.preparePrivateRequest(NewBalancesPayload(accountKey))
	balances, err := client.FetchBalances(ctx, suite.apikey, suite.secret, balancesRequest)
	suite.Require().NoError(err, "should not error during balances fetching")
	balance := findBalanceByCurrency(balances, "BAT")
	suite.Require().True(
		balance.Available.GreaterThanOrEqual(five),
		"must have at least 5 bat to pass the rest of the test",
	)

	tx := custodian.Transaction{
		// use this settlement id to create an ephemeral test
		SettlementID: uuid.NewV4().String(),
		Destination:  os.Getenv("GEMINI_TEST_DESTINATION_ID"),
		Channel:      "brave.com",
	}
	BAT := "BAT"
	payouts := []PayoutPayload{{
		TxRef:       GenerateTxRef(&tx),
		Amount:      five,
		Currency:    BAT,
		Destination: tx.Destination,
	}}
	bulkPayoutRequest := suite.preparePrivateRequest(NewBulkPayoutPayload(
		accountKey,
		os.Getenv("GEMINI_CLIENT_ID"),
		&payouts,
	))

	bulkPayoutResponse, err := client.UploadBulkPayout(ctx, suite.apikey, suite.secret, bulkPayoutRequest)
	pendingStatus := "Pending"
	expectedPayoutResult := PayoutResult{
		Result:      "OK",
		TxRef:       GenerateTxRef(&tx),
		Amount:      &five,
		Currency:    &BAT,
		Destination: &tx.Destination,
		Status:      &pendingStatus,
	}
	expectedPayoutResults := []PayoutResult{expectedPayoutResult}
	suite.Require().NoError(err, "should not error during bulk payout uploading")
	suite.Require().Equal(&expectedPayoutResults, bulkPayoutResponse, "the response should be predictable")

	status, err := client.CheckTxStatus(
		ctx,
		suite.apikey,
		os.Getenv("GEMINI_CLIENT_ID"),
		GenerateTxRef(&tx),
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

func (suite *GeminiTestSuite) preparePrivateRequest(payload interface{}) string {
	payloadSerialized, err := json.Marshal(payload)
	suite.Require().NoError(err, "payload must be able to be serialized")

	return base64.StdEncoding.EncodeToString(payloadSerialized)
}

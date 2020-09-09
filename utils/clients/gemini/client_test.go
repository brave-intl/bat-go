// +build integration,vpn

package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/utils/cryptography"
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

	// accountListRequest := suite.preparePrivateRequest(NewAccountListPayload())
	// accounts, err := client.FetchAccountList(ctx, suite.apikey, suite.secret, accountListRequest)
	// suite.Require().NoError(err, "should not error during account list fetching")
	// primary := "primary"
	// account := findAccountByClass(accounts, primary)
	// suite.Require().Equal(primary, account.Class, "should have a primary account")

	balancesRequest := suite.preparePrivateRequest(NewBalancesPayload())
	balances, err := client.FetchBalances(ctx, suite.apikey, suite.secret, balancesRequest)
	suite.Require().NoError(err, "should not error during balances fetching")
	balance := findBalanceByCurrency(balances, "BAT")
	suite.Require().True(
		balance.Available.GreaterThanOrEqual(five),
		"must have at least 5 bat to pass the rest of the test",
	)

	// tx := settlement.Transaction{
	// 	// use this settlement id to create an ephemeral test
	// 	// SettlementID: uuid.NewV4().String(),
	// 	SettlementID: uuid.Must(uuid.FromString("4077459f-7389-46d3-a0d8-b1e56b2d279b")).String(),
	// 	Destination:  os.Getenv("GEMINI_TEST_DESTINATION_ID"),
	// 	Channel:      "brave.com",
	// }
	// BAT := "BAT"
	// payouts := []PayoutPayload{{
	// 	TxRef:       GenerateTxRef(&tx),
	// 	Amount:      five,
	// 	Currency:    BAT,
	// 	Destination: tx.Destination,
	// }}
	// bulkPayoutRequest := suite.preparePrivateRequest(NewBulkPayoutPayload(
	// 	os.Getenv("GEMINI_CLIENT_ID"),
	// 	&payouts,
	// ))

	// _, err = client.UploadBulkPayout(ctx, suite.apikey, suite.secret, bulkPayoutRequest)
	// suite.Require().NoError(err, "should not error during bulk payout uploading")

	// pendingStatus := "Pending"
	// expectedPayoutResult := []PayoutResult{{
	// 	Result:      "OK",
	// 	TxRef:       GenerateTxRef(&tx),
	// 	Amount:      &five,
	// 	Currency:    &BAT,
	// 	Destination: &tx.Destination,
	// 	Status:      &pendingStatus,
	// }}
	// // suite.Require().Equal(&expectedPayoutResult, bulkPayoutResponse, "response should be predictable")
	// completeStatus := "Completed"
	// for {
	// 	<-time.After(5 * time.Second)
	// 	bulkPayoutRequest := suite.preparePrivateRequest(NewBulkPayoutPayload(
	// 		os.Getenv("GEMINI_CLIENT_ID"),
	// 		&payouts,
	// 	))

	// 	bulkPayoutResponse, err := client.UploadBulkPayout(ctx, suite.apikey, suite.secret, bulkPayoutRequest)
	// 	suite.Require().NoError(err, "should not error during bulk payout uploading")
	// 	if (*(*bulkPayoutResponse)[0].Status) == completeStatus {
	// 		// fmt.Printf("status: %s", *expectedPayoutResult[0].Status)
	// 		expectedPayoutResult[0].Status = &completeStatus
	// 		suite.Require().Equal(&expectedPayoutResult, bulkPayoutResponse, "success response should be predictable")
	// 		return
	// 	}
	// }
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

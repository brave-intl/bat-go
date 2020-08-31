// +build integration

package gemini

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/cryptography"
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

	accountListRequest := suite.preparePrivateRequest("hmac", NewAccountListPayload())
	accounts, err := client.FetchAccountList(ctx, accountListRequest)
	suite.Require().NoError(err, "should not error during account list fetching")
	primary := "primary"
	account := findAccountByClass(accounts, primary)
	suite.Require().Equal(primary, account.Class, "should have a primary account")

	balancesRequest := suite.preparePrivateRequest("hmac", NewBalancesPayload(&primary))
	balances, err := client.FetchBalances(ctx, balancesRequest)
	suite.Require().NoError(err, "should not error during balances fetching")
	balance := findBalanceByCurrency(balances, "BAT")
	five := decimal.NewFromFloat(5)
	suite.Require().True(
		balance.Available.GreaterThanOrEqual(five),
		"must have at least 5 bat to pass the rest of the test",
	)

	tx := settlement.Transaction{
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
		// Account:     primary,
	}}
	bulkPayoutRequest := suite.preparePrivateRequest("hmac", NewBulkPayoutPayload(
		primary,
		os.Getenv("GEMINI_CLIENT_ID"),
		&payouts,
	))

	bulkPayoutResponse, err := client.UploadBulkPayout(ctx, bulkPayoutRequest)
	suite.Require().NoError(err, "should not error during bulk payout uploading")

	pendingStatus := "Pending"
	expectedPayoutResult := []PayoutResult{{
		Result:      "OK",
		TxRef:       GenerateTxRef(&tx),
		Amount:      &five,
		Currency:    &BAT,
		Destination: &tx.Destination,
		Status:      &pendingStatus,
	}}
	suite.Require().Equal(&expectedPayoutResult, bulkPayoutResponse, "response should be predictable")
	// for {
	// 	<-time.After(5 * time.Second)
	// 	bulkPayoutRequest := suite.preparePrivateRequest("hmac", NewBulkPayoutPayload(
	// 		primary,
	// 		os.Getenv("GEMINI_CLIENT_ID"),
	// 		&payouts,
	// 	))

	// 	bulkPayoutResponse, err := client.UploadBulkPayout(ctx, bulkPayoutRequest)
	// 	suite.Require().NoError(err, "should not error during bulk payout uploading")
	// 	if (*(*bulkPayoutResponse)[0].Status) != pendingStatus {
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

func (suite *GeminiTestSuite) preparePrivateRequest(auth string, payload interface{}) PrivateRequest {
	payloadSerialized, err := json.Marshal(payload)
	suite.Require().NoError(err, "payload must be able to be serialized")

	payloadBase64 := base64.StdEncoding.EncodeToString(payloadSerialized)

	var signatureHex string
	if auth == "hmac" {
		signature, err := suite.secret.HMACSha384([]byte(payloadBase64))
		suite.Require().NoError(err, "payload must be able to be hashed")
		signatureHex = hex.EncodeToString(signature)
	}

	return PrivateRequest{
		Auth:      auth,
		Signature: signatureHex,
		Payload:   payloadBase64,
		APIKey:    suite.apikey,
	}
}

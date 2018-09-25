package grant

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/brave-intl/bat-go/datastore"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"github.com/pressly/lg"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

func TestFromCompactJWS(t *testing.T) {
	GrantSignatorPublicKeyHex = "f2eb37b5eb30ad5b888c680ab8848a46fc2a6be81324de990ad20dc9b6e569fe"
	registerGrantInstrumentation = false
	err := InitGrantService(nil)
	if err != nil {
		t.Error("unexpected error")
	}

	expectedGrantJSON := []byte(`{"altcurrency":"BAT","grantId":"9614ade7-58af-4df0-86c6-2f70051b43de","probi":"30000000000000000000","promotionId":"880309fc-df27-40a8-8d51-9cf39885e61d","maturityTime":1511769862,"expiryTime":1513843462}`)

	jwsGrant := "eyJhbGciOiJFZERTQSIsImtpZCI6IiJ9.eyJhbHRjdXJyZW5jeSI6IkJBVCIsImdyYW50SWQiOiI5NjE0YWRlNy01OGFmLTRkZjAtODZjNi0yZjcwMDUxYjQzZGUiLCJwcm9iaSI6IjMwMDAwMDAwMDAwMDAwMDAwMDAwIiwicHJvbW90aW9uSWQiOiI4ODAzMDlmYy1kZjI3LTQwYTgtOGQ1MS05Y2YzOTg4NWU2MWQiLCJtYXR1cml0eVRpbWUiOjE1MTE3Njk4NjIsImV4cGlyeVRpbWUiOjE1MTM4NDM0NjJ9.OOEBJUHPE21OyFw5Vq1tRTxYQc7aEL-KL5Lb4nb1TZn_3LFkXEPY7bNo0GhJ6k9X2UkZ19rnfBbpXHKuqBupDA"

	expectedGrant := Grant{}
	err = json.Unmarshal(expectedGrantJSON, &expectedGrant)
	if err != nil {
		t.Error("unexpected error")
	}

	grant, err := FromCompactJWS(grantPublicKey, jwsGrant)
	if err != nil {
		t.Error("unexpected error")
		t.Error(err)
	}

	if !reflect.DeepEqual(*grant, expectedGrant) {
		t.Error("grant not equal")
	}

	// TODO add incorrect signature tests
}

func TestVerifyAndConsume(t *testing.T) {
	GrantSignatorPublicKeyHex = "f03bccbcd2314d721f2375a669e7b817ef61067ab18a5da5df5b24b73feba8b7"
	SettlementDestination = "foo@bar.com"
	refreshBalance = false
	testSubmit = false
	registerGrantInstrumentation = false
	err := InitGrantService(nil)
	if err != nil {
		t.Error("unexpected error")
	}

	// Populate grant wallet balance to pass check
	oneHundred, err := decimal.NewFromString("100")
	if err != nil {
		t.Error(err)
	}
	grantWalletInfo := wallet.Info{}
	grantWalletInfo.LastBalance = &wallet.Balance{
		TotalProbi:       altcurrency.BAT.ToProbi(oneHundred),
		SpendableProbi:   altcurrency.BAT.ToProbi(oneHundred),
		ConfirmedProbi:   altcurrency.BAT.ToProbi(oneHundred),
		UnconfirmedProbi: decimal.Zero,
	}
	grantWalletInfo.Provider = "uphold"
	{
		tmp := altcurrency.BAT
		grantWalletInfo.AltCurrency = &tmp
	}
	grantWalletInfo.ProviderID = uuid.NewV4().String()

	grantWallet, err = uphold.FromWalletInfo(grantWalletInfo)
	if err != nil {
		t.Error(err)
	}

	grants := []string{"eyJhbGciOiJFZERTQSIsImtpZCI6IiJ9.eyJhbHRjdXJyZW5jeSI6IkJBVCIsImdyYW50SWQiOiIxOGY3Y2FkYS0yZTljLTRjNmUtYTU0MS1iMjAzMmM0M2E5MmUiLCJwcm9iaSI6IjMwMDAwMDAwMDAwMDAwMDAwMDAwIiwicHJvbW90aW9uSWQiOiJmNmQwNDg0Yy1kNzA5LTRjYTYtOWJhMS1lN2Q5MTI3YTQxOTAiLCJtYXR1cml0eVRpbWUiOjE1MTQ5MjM3MTMsImV4cGlyeVRpbWUiOjIyOTI2MTAxMTN9.haBAppDcMq0D1NlcxzBatwZGIKRtEGsw9_03PQtUAMTXmkc5LtoyFGgIXeZLIdrYmuowD8jHLIU3K1e8HJhzBA"}
	redeemedGrant := grants

	walletInfo := wallet.Info{}
	walletInfo.Provider = "uphold"
	{
		tmp := altcurrency.BAT
		walletInfo.AltCurrency = &tmp
	}

	walletInfo.ProviderID = uuid.NewV4().String()
	walletInfo.PublicKey = "424073b208e97af51cab7a389bcfe6942a3b7c7520fe9dab84f311f7846f5fcf"
	walletInfo.LastBalance = &wallet.Balance{}

	transaction := "eyJoZWFkZXJzIjp7ImRpZ2VzdCI6IlNIQS0yNTY9WFg0YzgvM0J4ejJkZWNkakhpY0xWaXJ5dTgxbWdGNkNZTTNONFRHc0xoTT0iLCJzaWduYXR1cmUiOiJrZXlJZD1cInByaW1hcnlcIixhbGdvcml0aG09XCJlZDI1NTE5XCIsaGVhZGVycz1cImRpZ2VzdFwiLHNpZ25hdHVyZT1cIjI4TitabzNodlRRWmR2K2trbGFwUE5IY29OMEpLdWRiSU5GVnlOSm0rWDBzdDhzbXdzYVlHaTJQVHFRbjJIVWdacUp4Q2NycEpTMWpxZHdyK21RNEN3PT1cIiJ9LCJvY3RldHMiOiJ7XCJkZW5vbWluYXRpb25cIjp7XCJhbW91bnRcIjpcIjI1XCIsXCJjdXJyZW5jeVwiOlwiQkFUXCJ9LFwiZGVzdGluYXRpb25cIjpcImZvb0BiYXIuY29tXCJ9In0="

	grantID := "18f7cada-2e9c-4c6e-a541-b2032c43a92e"

	logger := logrus.New()
	ctx := context.Background()
	ctx = lg.WithLoggerContext(ctx, logger)

	claimReq := ClaimGrantRequest{WalletInfo: walletInfo}
	err = claimReq.Claim(ctx, grantID)
	if err != nil {
		t.Error("Claim failed")
	}

	// Claim and redeem request are for different wallets, should fail
	walletInfo.ProviderID = uuid.NewV4().String()
	request := RedeemGrantsRequest{Grants: grants, WalletInfo: walletInfo, Transaction: transaction}

	_, err = request.VerifyAndConsume(ctx)
	if err == nil {
		t.Error("Should not be able to redeem a non-matching claim on a grant")
		return
	}

	kvDatastore, err := datastore.GetKvDatastore(ctx)
	if err != nil {
		t.Error(err)
	}
	// Clear out previous claim
	_, err = kvDatastore.Delete(fmt.Sprintf(claimKeyFormat, grantID))
	if err != nil {
		t.Error(err)
	}

	claimReq = ClaimGrantRequest{WalletInfo: walletInfo}
	err = claimReq.Claim(ctx, grantID)
	if err != nil {
		t.Error("Claim failed")
	}

	_, err = request.VerifyAndConsume(ctx)
	if err != nil {
		t.Error(err)
	}

	_, err = request.VerifyAndConsume(ctx)
	if err == nil {
		t.Error("expected re-redeem with the same grant to fail")
	}

	// another grant from the same promotionId
	grants = []string{"eyJhbGciOiJFZERTQSIsImtpZCI6IiJ9.eyJhbHRjdXJyZW5jeSI6IkJBVCIsImdyYW50SWQiOiI1MzZjNWZhZC0zYWJiLTQwM2UtOWI5Mi1kNjE5ZDc0YjNhZjQiLCJwcm9iaSI6IjMwMDAwMDAwMDAwMDAwMDAwMDAwIiwicHJvbW90aW9uSWQiOiJmNmQwNDg0Yy1kNzA5LTRjYTYtOWJhMS1lN2Q5MTI3YTQxOTAiLCJtYXR1cml0eVRpbWUiOjE1MTQ5MjM3MTMsImV4cGlyeVRpbWUiOjIyOTI2MTAxMTN9.Y5QruXFJVV0qqRauP3ah4UAHk6TgtNPySkbq3VBv3dCKpAvYmSnfBRipKjVCicP2s0lQQn8Rcu3aIP4VDBCjDQ"}

	// claim this grant as well to ensure we are testing re-redeem with same wallet and promotion
	err = claimReq.Claim(ctx, "f87e7fb4-0f80-40ad-b092-84f70e448421")
	if err != nil {
		t.Error("Claim failed")
	}

	request = RedeemGrantsRequest{Grants: grants, WalletInfo: walletInfo, Transaction: transaction}
	_, err = request.VerifyAndConsume(ctx)
	if err == nil {
		t.Error("expected re-redeem with the same wallet and promotion to fail")
	}

	redeemedIDs, err := GetRedeemedIDs(ctx, redeemedGrant)
	if err != nil {
		t.Error("Unable to check RedeemedIDs")
	}
	expectedRedeemedIDs := []string{grantID}
	if !reflect.DeepEqual(redeemedIDs, expectedRedeemedIDs) {
		t.Error("IDs do not match")
	}
	// TODO add more tests
}

func TestByExpiryTimestamp(t *testing.T) {
	grants := []Grant{{ExpiryTimestamp: 12345}, {ExpiryTimestamp: 1234}}
	sort.Sort(ByExpiryTimestamp(grants))
	var last int64
	for _, grant := range grants {
		if grant.ExpiryTimestamp < last {
			t.Error("Order is not correct")
			last = grant.ExpiryTimestamp
		}
	}
}

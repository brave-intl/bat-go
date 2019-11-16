package grant

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

func TestFromCompactJWS(t *testing.T) {
	GrantSignatorPublicKeyHex = "f2eb37b5eb30ad5b888c680ab8848a46fc2a6be81324de990ad20dc9b6e569fe"
	registerGrantInstrumentation = false
	_, err := InitService(nil)
	if err != nil {
		t.Fatal("unexpected error", err)
	}

	expectedGrantJSON := []byte(`{"altcurrency":"BAT","grantId":"9614ade7-58af-4df0-86c6-2f70051b43de","probi":"30000000000000000000","promotionId":"880309fc-df27-40a8-8d51-9cf39885e61d","maturityTime":1511769862,"expiryTime":1513843462}`)

	jwsGrant := "eyJhbGciOiJFZERTQSIsImtpZCI6IiJ9.eyJhbHRjdXJyZW5jeSI6IkJBVCIsImdyYW50SWQiOiI5NjE0YWRlNy01OGFmLTRkZjAtODZjNi0yZjcwMDUxYjQzZGUiLCJwcm9iaSI6IjMwMDAwMDAwMDAwMDAwMDAwMDAwIiwicHJvbW90aW9uSWQiOiI4ODAzMDlmYy1kZjI3LTQwYTgtOGQ1MS05Y2YzOTg4NWU2MWQiLCJtYXR1cml0eVRpbWUiOjE1MTE3Njk4NjIsImV4cGlyeVRpbWUiOjE1MTM4NDM0NjJ9.OOEBJUHPE21OyFw5Vq1tRTxYQc7aEL-KL5Lb4nb1TZn_3LFkXEPY7bNo0GhJ6k9X2UkZ19rnfBbpXHKuqBupDA"

	expectedGrant := Grant{}
	err = json.Unmarshal(expectedGrantJSON, &expectedGrant)
	if err != nil {
		t.Fatal("unexpected error", err)
	}

	grant, err := FromCompactJWS(grantPublicKey, jwsGrant)
	if err != nil {
		t.Fatal("unexpected error", err)
	}

	if !reflect.DeepEqual(*grant, expectedGrant) {
		t.Fatal("grant not equal", err)
	}

	// TODO add incorrect signature tests
}

func TestConsume(t *testing.T) {
	GrantSignatorPublicKeyHex = "f03bccbcd2314d721f2375a669e7b817ef61067ab18a5da5df5b24b73feba8b7"
	SettlementDestination = "foo@bar.com"
	refreshBalance = false
	testSubmit = false
	registerGrantInstrumentation = false

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDB := NewMockDatastore(mockCtrl)

	service, err := InitService(mockDB)
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

	grantID, _ := uuid.FromString("18f7cada-2e9c-4c6e-a541-b2032c43a92e")
	var grant Grant
	grant.GrantID = grantID
	{
		tmp := altcurrency.BAT
		grant.AltCurrency = &tmp
	}
	grant.Probi = grant.AltCurrency.ToProbi(decimal.NewFromFloat(25))
	grant.ExpiryTimestamp = time.Date(2020, time.November, 10, 23, 0, 0, 0, time.UTC).Unix()

	ctx := context.Background()

	mockDB.EXPECT().UpsertWallet(gomock.Eq(&walletInfo)).Return(nil)
	mockDB.EXPECT().ClaimGrantForWallet(gomock.Eq(grant), gomock.Eq(walletInfo)).Return(nil)
	err = service.Claim(ctx, walletInfo, grant)
	if err != nil {
		t.Error("Claim failed")
	}

	request := RedeemGrantsRequest{WalletInfo: walletInfo, Transaction: transaction}

	mockDB.EXPECT().GetGrantsOrderedByExpiry(gomock.Eq(walletInfo), gomock.Eq("")).Return([]Grant{grant}, nil)
	mockDB.EXPECT().RedeemGrantForWallet(gomock.Eq(grant), gomock.Eq(walletInfo)).Return(nil)
	_, err = service.Consume(ctx, request.WalletInfo, request.Transaction)
	if err != nil {
		t.Error(err)
	}

	mockDB.EXPECT().GetGrantsOrderedByExpiry(gomock.Eq(walletInfo), gomock.Eq("")).Return([]Grant{}, nil)
	txnInfo, err := service.Consume(ctx, request.WalletInfo, request.Transaction)
	if err != nil {
		t.Error(err)
	}
	if txnInfo != nil {
		t.Error("expected redeem without grants to return no transaction")
	}
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

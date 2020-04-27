package grant

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	promotion "github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

func TestConsume(t *testing.T) {
	GrantSignatorPublicKeyHex = "f03bccbcd2314d721f2375a669e7b817ef61067ab18a5da5df5b24b73feba8b7"
	refreshBalance = false
	testSubmit = false
	registerGrantInstrumentation = false

	uphold.SettlementDestination = "foo@bar.com"
	oldUpholdSettlementAddress := uphold.UpholdSettlementAddress
	uphold.UpholdSettlementAddress = "foo@bar.com"
	oldAnonCardSettlementAddress := uphold.AnonCardSettlementAddress
	uphold.AnonCardSettlementAddress = "foo@bar.com"
	defer func() {
		uphold.UpholdSettlementAddress = oldUpholdSettlementAddress
		uphold.AnonCardSettlementAddress = oldAnonCardSettlementAddress
	}()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDB := NewMockDatastore(mockCtrl)

	err := os.Setenv("ENV", "local")
	if err != nil {
		t.Fatal("unexpected error", err)
	}
	service, err := InitService(mockDB, nil)
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
	promotionID := uuid.NewV4()
	var grant Grant
	grant.GrantID = grantID
	grant.PromotionID = promotionID
	{
		tmp := altcurrency.BAT
		grant.AltCurrency = &tmp
	}
	grant.Probi = grant.AltCurrency.ToProbi(decimal.NewFromFloat(25))
	grant.ExpiryTimestamp = time.Date(2020, time.November, 10, 23, 0, 0, 0, time.UTC).Unix()

	ctx := context.Background()

	promo := &promotion.Promotion{Type: "ads", ID: promotionID}

	mockDB.EXPECT().UpsertWallet(gomock.Eq(&walletInfo)).Return(nil)
	mockDB.EXPECT().GetPromotion(gomock.Eq(promotionID)).Return(promo, nil)
	mockDB.EXPECT().ClaimPromotionForWallet(gomock.Eq(promo), gomock.Eq(&walletInfo)).Return(&promotion.Claim{}, nil)
	_, err = service.ClaimPromotion(ctx, walletInfo, grant.PromotionID)
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

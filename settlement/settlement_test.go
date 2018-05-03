package settlement

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"golang.org/x/crypto/ed25519"
)

func TestTransactions(t *testing.T) {
	if os.Getenv("UPHOLD_ACCESS_TOKEN") == "" {
		t.Skip("skipping test; UPHOLD_ACCESS_TOKEN not set")
	}
	if os.Getenv("DONOR_WALLET_PUBLIC_KEY") == "" {
		t.Skip("skipping test; DONOR_WALLET_PUBLIC_KEY not set")
	}
	if os.Getenv("DONOR_WALLET_PRIVATE_KEY") == "" {
		t.Skip("skipping test; DONOR_WALLET_PRIVATE_KEY not set")
	}
	if os.Getenv("DONOR_WALLET_CARD_ID") == "" {
		t.Skip("skipping test; DONOR_WALLET_CARD_ID not set")
	}
	if os.Getenv("QA_PUBLISHER_USD_CARD_ID") == "" {
		t.Skip("skipping test; QA_PUBLISHER_USD_CARD_ID not set")
	}

	usdCard := os.Getenv("QA_PUBLISHER_USD_CARD_ID")

	var donorInfo wallet.Info
	donorInfo.Provider = "uphold"
	donorInfo.ProviderID = os.Getenv("DONOR_WALLET_CARD_ID")
	{
		tmp := altcurrency.BAT
		donorInfo.AltCurrency = &tmp
	}

	donorWalletPublicKeyHex := os.Getenv("DONOR_WALLET_PUBLIC_KEY")
	donorWalletPrivateKeyHex := os.Getenv("DONOR_WALLET_PRIVATE_KEY")
	var donorPublicKey httpsignature.Ed25519PubKey
	var donorPrivateKey ed25519.PrivateKey
	donorPublicKey, err := hex.DecodeString(donorWalletPublicKeyHex)
	if err != nil {
		t.Fatal(err)
	}
	donorPrivateKey, err = hex.DecodeString(donorWalletPrivateKeyHex)
	if err != nil {
		t.Fatal(err)
	}
	donorWallet := &uphold.Wallet{donorInfo, donorPrivateKey, donorPublicKey}

	if len(donorWallet.ID) > 0 {
		t.Fatal("FIXME")
	}

	settlementJson := []byte(`
	[
    {
        "address": "` + usdCard + `",
        "altcurrency": "BAT",
        "authority": "github:evq",
        "currency": "BAT",
        "fees": "1339169009771847163",
        "owner": "publishers#uuid:23813236-3f4c-40dc-916e-8f55c8865b5a",
        "probi": "25444211185665096101",
        "publisher": "example.com",
        "transactionId": "0f7377cc-73ef-4e94-b69a-7086a4f3b2a8",
        "type": "referral"
    }
	]
	`)

	var settlements []SettlementTransaction
	err = json.Unmarshal(settlementJson, &settlements)
	if err != nil {
		t.Fatal(err)
	}

	err = PrepareSettlementTransactions(donorWallet, settlements)
	if err != nil {
		t.Fatal(err)
	}

	err = SubmitPreparedTransactions(donorWallet, settlements)
	if err != nil {
		t.Fatal(err)
	}

	var hashes []string
	for i := 0; i < len(settlements); i++ {
		hashes = append(hashes, settlements[i].ProviderID)
	}

	// Multiple submit should have no effect
	err = SubmitPreparedTransactions(donorWallet, settlements)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < len(settlements); i++ {
		if hashes[i] != settlements[i].ProviderID {
			t.Fatal("Hash for settlement failed")
		}
	}

	err = ConfirmPreparedTransactions(donorWallet, settlements)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < len(settlements); i++ {
		txInfo, err := donorWallet.GetTransaction(settlements[i].ProviderID)
		if err != nil {
			t.Fatal(err)
		}
		if err = checkTransactionAgainstSettlement(&settlements[i], txInfo); err != nil {
			t.Error("Uphold transaction referenced in settlement should match!")
			t.Fatal(err)
		}
	}

	// Multiple confirm should not error
	err = ConfirmPreparedTransactions(donorWallet, settlements)
	if err != nil {
		t.Fatal(err)
	}
}

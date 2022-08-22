package settlement

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/ed25519"
)

func TestTransactions(t *testing.T) {
	ctx := context.Background()

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
	donorWallet := &uphold.Wallet{Info: donorInfo, PrivKey: donorPrivateKey, PubKey: donorPublicKey}

	if len(donorWallet.ID) > 0 {
		t.Fatal("FIXME")
	}

	settlementJSON := []byte(`
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

	var settlements []custodian.Transaction
	err = json.Unmarshal(settlementJSON, &settlements)
	if err != nil {
		t.Fatal(err)
	}

	err = PrepareTransactions(donorWallet, settlements, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	err = SubmitPreparedTransactions(ctx, donorWallet, settlements)
	if err != nil {
		t.Fatal(err)
	}

	var hashes []string
	for i := 0; i < len(settlements); i++ {
		hashes = append(hashes, settlements[i].ProviderID)
	}

	// Multiple submit should have no effect
	err = SubmitPreparedTransactions(ctx, donorWallet, settlements)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < len(settlements); i++ {
		if hashes[i] != settlements[i].ProviderID {
			t.Fatal("Hash for settlement failed")
		}
	}

	err = ConfirmPreparedTransactions(ctx, donorWallet, settlements)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < len(settlements); i++ {
		var txInfo *wallet.TransactionInfo
		txInfo, err = donorWallet.GetTransaction(ctx, settlements[i].ProviderID)
		if err != nil {
			t.Fatal(err)
		}
		if err = checkTransactionAgainstSettlement(&settlements[i], txInfo); err != nil {
			t.Error("Uphold transaction referenced in settlement should match!")
			t.Fatal(err)
		}
	}

	// Multiple confirm should not error
	err = ConfirmPreparedTransactions(ctx, donorWallet, settlements)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeterministicSigning(t *testing.T) {
	usdCard := "03aeafb8-555d-4840-90d1-ff0f99426475"

	var donorInfo wallet.Info
	donorInfo.Provider = "uphold"
	donorInfo.ProviderID = "aea53308-9b35-4f63-bccd-9f1dffa3d8c0"
	{
		tmp := altcurrency.BAT
		donorInfo.AltCurrency = &tmp
	}

	donorWalletPublicKeyHex := "10ba999b2b7b9eabc0f44fa26bf122ebbfa98dc6fef31e6251a9c1c58d60bb8d"
	donorWalletPrivateKeyHex := "8d6a620a566e094cebaec67edca32a68efce962890570157f0b8a5389cc5f6df10ba999b2b7b9eabc0f44fa26bf122ebbfa98dc6fef31e6251a9c1c58d60bb8d"
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
	donorWallet := &uphold.Wallet{Info: donorInfo, PrivKey: donorPrivateKey, PubKey: donorPublicKey}

	if len(donorWallet.ID) > 0 {
		t.Fatal("FIXME")
	}

	settlementJSON := []byte(`
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

	bravePaymentToolsSignedSettlement := []byte(`
	{
		"config": {
			"env": "test"
		},
		"signedTxs": [
			{
				"id": "f042845f-fa62-4022-8117-a476ec583a7b",
				"requestType": "httpSignature",
				"signedTx": {
					"headers": {
						"digest": "SHA-256=zrtB9DhyDmPLMml/JwBJ3rnVyzBYhBGgoYiGaL5msYI=",
						"signature": "keyId=\"primary\",algorithm=\"ed25519\",headers=\"digest\",signature=\"1n4soEhMbhhHHk2IZ9xkVsaFRj9ajD6+y4MEzl8FcxZTviy5utHIKugPiFMQvSaktegvA5NIs3wNGFsuk4OtBQ==\""
					},
					"body": {
						"denomination": {
							"amount": "25.444211185665096101",
							"currency": "BAT"
						},
						"destination": "03aeafb8-555d-4840-90d1-ff0f99426475",
						"message": "0f7377cc-73ef-4e94-b69a-7086a4f3b2a8"
					},
					"octets": "{\"denomination\":{\"amount\":\"25.444211185665096101\",\"currency\":\"BAT\"},\"destination\":\"03aeafb8-555d-4840-90d1-ff0f99426475\",\"message\":\"0f7377cc-73ef-4e94-b69a-7086a4f3b2a8\"}"
				}
			}
		],
		"authenticate": {}
	}
  `)

	knownSigs, err := ParseBPTSignedSettlement(bravePaymentToolsSignedSettlement)
	if err != nil {
		t.Fatal(err)
	}

	var settlements []custodian.Transaction
	err = json.Unmarshal(settlementJSON, &settlements)
	if err != nil {
		t.Fatal(err)
	}

	err = PrepareTransactions(donorWallet, settlements, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range settlements {
		if settlements[i].SignedTx != knownSigs[i] {
			t.Fatal("Signature does not match equivalent from brave-payment-tools")
		}
	}
}

func TestUnmarshalAdsTransaction(t *testing.T) {
	settlementJSON := []byte(`
  [
    {
        "address": "5e14c5b2-8651-427d-905e-b078513b6fc3",
        "altcurrency": "BAT",
        "currency": "BAT",
        "fees": "0",
        "note": "payout for test ad earnings",
        "probi": "25444211185665096101",
				"publisher": "wallet:6e4111a8-0946-4dff-8010-65272bc5b7bb",
        "transactionId": "0f7377cc-73ef-4e94-b69a-7086a4f3b2a8",
        "type": "adsDirectDeposit",
				"walletProvider": "uphold",
				"walletProviderId": "6ff1c5af-5943-4e01-bc2c-88a8881c3606"
    },
    {
        "address": "5e14c5b2-8651-427d-905e-b078513b6fc3",
        "altcurrency": "BAT",
        "currency": "BAT",
        "fees": "0",
        "note": "payout for test ad earnings",
        "probi": "25444211185665096101",
				"publisher": "wallet:6e4111a8-0946-4dff-8010-65272bc5b7bb",
        "transactionId": "0f7377cc-73ef-4e94-b69a-7086a4f3b2a8",
        "type": "adsDirectDeposit",
				"walletProvider": "uphold",
				"walletProviderId": "6ff1c5af-5943-4e01-bc2c-88a8881c3606"
    }
  ]
  `)

	var afTransactions []AntifraudTransaction
	err := json.Unmarshal(settlementJSON, &afTransactions)
	if err != nil {
		t.Fatal(err)
	}

	var settlements []custodian.Transaction
	for _, afTxn := range afTransactions {
		txn, err := afTxn.ToTransaction()
		if err != nil {
			t.Fatal(err)
		}
		settlements = append(settlements, txn)
	}

	expected, err := decimal.NewFromString("25.444211185665096101")
	if err != nil {
		t.Fatal(err)
	}
	if !settlements[0].Amount.Equals(expected) {
		t.Fatal("Amount does not match")
	}
}

func TestUnmarshalCreatorsTransaction(t *testing.T) {
	settlementJSON := []byte(`
	{
			"address": "5e14c5b2-8651-427d-905e-b078513b6fc3",
			"bat": "0.712500000000000000",
			"channel_type": "TwitchChannelDetails",
			"created_at": "2021-11-09 02:20:47.586946+00:00",
			"fees": "0.037500000000000006",
			"id": "c6d5983b-fef7-4e7d-b0a3-5bf68e0bd95a",
			"inserted_at": "2021-11-08 21:53:30.146504+00:00",
			"owner": "publishers#uuid:008c092b-7afa-47f0-9677-22538d9abc3d",
			"owner_state": "active",
			"payout_report_id": "4520e913-664e-479e-a58c-357cf750b00a",
			"publisher": "twitch#author:meliunu",
			"type": "contribution",
			"url": "https://twitch.tv/meliunu",
			"wallet_country_code": "CL",
			"wallet_provider": "0",
			"wallet_provider_id": "uphold#id:6c0397f3-df41-440a-9fbb-b517e1142a9a"
	}
  `)

	var settlement AntifraudTransaction
	err := json.Unmarshal(settlementJSON, &settlement)
	if err != nil {
		t.Fatal(err)
	}

	txn, err := settlement.ToTransaction()
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.MarshalIndent(txn, "", "    ")
	if err != nil {
		t.Fatal(err)
	}

	expected := []byte(`{
    "altcurrency": "BAT",
    "authority": "",
    "amount": "0.7125",
    "commission": "0",
    "currency": "BAT",
    "address": "5e14c5b2-8651-427d-905e-b078513b6fc3",
    "owner": "publishers#uuid:008c092b-7afa-47f0-9677-22538d9abc3d",
    "fees": "37500000000000006",
    "probi": "712500000000000000",
    "hash": "",
    "walletProvider": "uphold",
    "walletProviderId": "6c0397f3-df41-440a-9fbb-b517e1142a9a",
    "publisher": "twitch#author:meliunu",
    "signedTx": "",
    "status": "",
    "transactionId": "4520e913-664e-479e-a58c-357cf750b00a",
    "fee": "0",
    "type": "contribution",
    "validUntil": "0001-01-01T00:00:00Z",
    "note": ""
}`)

	if string(out) != string(expected) {
		t.Fatal("Converted transaction does not match")
	}
}

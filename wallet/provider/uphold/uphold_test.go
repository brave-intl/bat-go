package uphold

import (
	"encoding/hex"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

func TestGetCardDetails(t *testing.T) {
	if os.Getenv("UPHOLD_ACCESS_TOKEN") == "" {
		t.Skip("skipping test; UPHOLD_ACCESS_TOKEN not set")
	}

	var info wallet.Info
	info.Provider = "uphold"
	info.ProviderID = "6654ecb0-6079-4f6c-ba58-791cc890a561"
	{
		tmp := altcurrency.BAT
		info.AltCurrency = &tmp
	}

	wallet, err := FromWalletInfo(info)
	if err != nil {
		t.Error(err)
	}
	_, err = wallet.GetBalance(true)
	if err != nil {
		t.Error(err)
	}
}

func TestRegister(t *testing.T) {
	if os.Getenv("UPHOLD_ACCESS_TOKEN") == "" {
		t.Skip("skipping test; UPHOLD_ACCESS_TOKEN not set")
	}

	var info wallet.Info
	info.Provider = "uphold"
	info.ProviderID = ""
	{
		tmp := altcurrency.BAT
		info.AltCurrency = &tmp
	}

	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}

	wallet := &Wallet{info, privateKey, publicKey}
	err = wallet.Register("bat-go test card")
	if err != nil {
		t.Error(err)
	}
	fmt.Println("> " + wallet.Info.ProviderID)
}

func TestDecodeTransaction(t *testing.T) {
	var info wallet.Info
	info.Provider = "uphold"
	info.ProviderID = uuid.NewV4().String()
	{
		tmp := altcurrency.BAT
		info.AltCurrency = &tmp
	}

	wallet, err := FromWalletInfo(info)
	if err != nil {
		t.Error(err)
	}

	var pk httpsignature.Ed25519PubKey
	pk, _ = hex.DecodeString("424073b208e97af51cab7a389bcfe6942a3b7c7520fe9dab84f311f7846f5fcf")
	wallet.PubKey = pk

	txnB64 := "eyJoZWFkZXJzIjp7ImRpZ2VzdCI6IlNIQS0yNTY9WFg0YzgvM0J4ejJkZWNkakhpY0xWaXJ5dTgxbWdGNkNZTTNONFRHc0xoTT0iLCJzaWduYXR1cmUiOiJrZXlJZD1cInByaW1hcnlcIixhbGdvcml0aG09XCJlZDI1NTE5XCIsaGVhZGVycz1cImRpZ2VzdFwiLHNpZ25hdHVyZT1cIjI4TitabzNodlRRWmR2K2trbGFwUE5IY29OMEpLdWRiSU5GVnlOSm0rWDBzdDhzbXdzYVlHaTJQVHFRbjJIVWdacUp4Q2NycEpTMWpxZHdyK21RNEN3PT1cIiJ9LCJvY3RldHMiOiJ7XCJkZW5vbWluYXRpb25cIjp7XCJhbW91bnRcIjpcIjI1XCIsXCJjdXJyZW5jeVwiOlwiQkFUXCJ9LFwiZGVzdGluYXRpb25cIjpcImZvb0BiYXIuY29tXCJ9In0="

	txnReq, err := wallet.decodeTransaction(txnB64)
	if err != nil {
		t.Error(err)
	}

	var expected transactionRequest
	expected.Destination = "foo@bar.com"
	expected.Denomination.Amount, _ = decimal.NewFromString("25.0")
	{
		tmp := altcurrency.BAT
		expected.Denomination.Currency = &tmp
	}

	if !reflect.DeepEqual(*txnReq, expected) {
		t.Error("Decoded transaction does not match expected value")
	}
}

func TestReMarshall(t *testing.T) {
	// FIXME
	//{"denomination":{"amount":"50.000000000000000000","currency":"BAT"},"destination":"99f7ee1c-bce7-4b11-bb91-825412f4764b"}}
}

func TestVerifyTransaction(t *testing.T) {
	// FIXME test malicious signature cases
}

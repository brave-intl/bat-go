package uphold

import (
	"encoding/hex"
	"fmt"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"os"
	"reflect"
	"testing"
)

func TestGetCardDetails(t *testing.T) {
	if os.Getenv("UPHOLD_ACCESS_TOKEN") == "" {
		t.Skip("skipping test; UPHOLD_ACCESS_TOKEN not set")
	}

	var info wallet.WalletInfo
	info.Provider = "uphold"
	//info.ProviderId = "3f9bfeb9-bafd-43b7-ac64-4e47208ff1a1"
	info.ProviderId = "6654ecb0-6079-4f6c-ba58-791cc890a561"
	info.AltCurrency = altcurrency.BAT

	wallet, err := FromWalletInfo(info)
	if err != nil {
		t.Error(err)
	}
	details, err := wallet.GetBalance(true)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(details)
}

func TestDecodeTransaction(t *testing.T) {
	var info wallet.WalletInfo
	info.Provider = "uphold"
	info.ProviderId = uuid.NewV4().String()
	info.AltCurrency = altcurrency.BAT

	wallet, err := FromWalletInfo(info)
	if err != nil {
		t.Error(err)
	}

	wallet.PubKey, _ = hex.DecodeString("656c44ebe9640d5491bbe2b4a29efd0775f4ee73002681b4caf604bd55482da6")

	txnB64 := "eyJoZWFkZXJzIjp7ImRpZ2VzdCI6IlNIQS0yNTY9WG9sTTVjUVhCU055Vmc1M0NrQjROU215b0h0dnJOVysyYTdWVTB0RmpMaz0iLCJzaWduYXR1cmUiOiJrZXlJZD1cInByaW1hcnlcIixhbGdvcml0aG09XCJlZDI1NTE5XCIsaGVhZGVycz1cImRpZ2VzdFwiLHNpZ25hdHVyZT1cIlpHU1U0RlYyN0RSZjJOYmh3UFkwb0ZGcFRlcURuM0VHZkpmYkQ4MzZkdWFqdWVKOTMvemg2YTF0SUtONklTTWFFTXMwRjFiUnhPWjVxSkIwK256V0JRPT1cIiJ9LCJvY3RldHMiOiJ7XCJkZW5vbWluYXRpb25cIjp7XCJhbW91bnRcIjoyNSxcImN1cnJlbmN5XCI6XCJCQVRcIn0sXCJkZXN0aW5hdGlvblwiOlwiZm9vQGJhci5jb21cIn0ifQ=="

	txnReq, err := wallet.DecodeTransaction(txnB64)
	if err != nil {
		t.Error(err)
	}

	var expected TransactionRequest
	expected.Destination = "foo@bar.com"
	expected.Denomination.Amount, _ = decimal.NewFromString("25.0")
	expected.Denomination.Currency = altcurrency.BAT

	if !reflect.DeepEqual(*txnReq, expected) {
		t.Error("Decoded transaction does not match expected value")
	}
}

func TestVerifyTransaction(t *testing.T) {
	// FIXME test malicious signature cases
}

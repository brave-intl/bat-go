package uphold

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/pindialer"
	"github.com/brave-intl/bat-go/libs/wallet"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/ed25519"
	"gotest.tools/assert"
)

func TestGetCardDetails(t *testing.T) {
	ctx := context.Background()

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

	wallet, err := FromWalletInfo(ctx, info)
	if err != nil {
		t.Error(err)
	}
	_, err = wallet.GetBalance(ctx, true)
	if err != nil {
		t.Error(err)
	}
}

func TestRegister(t *testing.T) {
	ctx := context.Background()

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

	destWallet := &Wallet{Info: info, PrivKey: privateKey, PubKey: publicKey}
	err = destWallet.Register(ctx, "bat-go test card")
	if err != nil {
		t.Error(err)
	}
}

func TestDecodeTransaction(t *testing.T) {
	var info wallet.Info
	info.Provider = "uphold"
	info.ProviderID = uuid.NewV4().String()
	{
		tmp := altcurrency.BAT
		info.AltCurrency = &tmp
	}

	wallet, err := FromWalletInfo(context.Background(), info)
	if err != nil {
		t.Error(err)
	}

	var pk httpsignature.Ed25519PubKey
	pk, err = hex.DecodeString("424073b208e97af51cab7a389bcfe6942a3b7c7520fe9dab84f311f7846f5fcf")
	if err != nil {
		t.Error(err)
	}
	wallet.PubKey = pk

	txnB64 := "eyJoZWFkZXJzIjp7ImRpZ2VzdCI6IlNIQS0yNTY9WFg0YzgvM0J4ejJkZWNkakhpY0xWaXJ5dTgxbWdGNkNZTTNONFRHc0xoTT0iLCJzaWduYXR1cmUiOiJrZXlJZD1cInByaW1hcnlcIixhbGdvcml0aG09XCJlZDI1NTE5XCIsaGVhZGVycz1cImRpZ2VzdFwiLHNpZ25hdHVyZT1cIjI4TitabzNodlRRWmR2K2trbGFwUE5IY29OMEpLdWRiSU5GVnlOSm0rWDBzdDhzbXdzYVlHaTJQVHFRbjJIVWdacUp4Q2NycEpTMWpxZHdyK21RNEN3PT1cIiJ9LCJvY3RldHMiOiJ7XCJkZW5vbWluYXRpb25cIjp7XCJhbW91bnRcIjpcIjI1XCIsXCJjdXJyZW5jeVwiOlwiQkFUXCJ9LFwiZGVzdGluYXRpb25cIjpcImZvb0BiYXIuY29tXCJ9In0="

	txnReq, err := wallet.decodeTransaction(txnB64)
	if err != nil {
		t.Error(err)
	}

	var expected transactionRequest
	expected.Destination = "foo@bar.com"
	expected.Denomination.Amount, err = decimal.NewFromString("25.0")
	if err != nil {
		t.Error(err)
	}
	{
		tmp := altcurrency.BAT
		expected.Denomination.Currency = &tmp
	}

	if txnReq.Destination != expected.Destination {
		t.Error("Decoded transaction does not match expected value")
	}
	if !txnReq.Denomination.Amount.Equal(expected.Denomination.Amount) {
		t.Error("Decoded transaction does not match expected value")
	}
	if *txnReq.Denomination.Currency != *expected.Denomination.Currency {
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
	donorWallet := &Wallet{Info: donorInfo, PrivKey: donorPrivateKey, PubKey: donorPublicKey}

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

	destWallet := &Wallet{Info: info, PrivKey: privateKey, PubKey: publicKey}
	err = destWallet.Register(ctx, "bat-go test transaction card")
	if err != nil {
		t.Error(err)
	}

	value, err := decimal.NewFromString("10")
	if err != nil {
		t.Error(err)
	}

	tx, err := donorWallet.PrepareTransaction(
		altcurrency.BAT,
		altcurrency.BAT.ToProbi(value),
		destWallet.Info.ProviderID,
		"bat-go:uphold.TestTransactions",
		"",
		nil,
	)
	if err != nil {
		t.Error(err)
	}

	submitInfo, err := donorWallet.SubmitTransaction(ctx, tx, false)
	if err != nil {
		t.Error(err)
	}

	balance, err := destWallet.GetBalance(ctx, true)
	if err != nil {
		t.Error(err)
	}

	if balance.TotalProbi.GreaterThan(decimal.Zero) {
		t.Error("Submit without confirm should not result in a balance.")
	}

	// Submitted but unconfirmed transactions cannot be retrieved via GetTransaction
	_, err = donorWallet.GetTransaction(ctx, submitInfo.ID)
	if err == nil {
		t.Error("Expected error retrieving unconfirmed transaction")
	}
	if !errorutils.IsErrNotFound(err) {
		t.Error("Expected \"missing\" transaction as error cause")
	}

	commitInfo, err := donorWallet.ConfirmTransaction(ctx, submitInfo.ID)
	if err != nil {
		t.Error(err)
	}

	if commitInfo.ID != submitInfo.ID {
		t.Error("Transaction id mismatch!")
	}

	if commitInfo.Destination != destWallet.ProviderID {
		t.Error("Transaction destination mismatch!")
	}

	if !commitInfo.Probi.Equals(submitInfo.Probi) {
		t.Error("Transaction probi mismatch!")
	}

	getInfo, err := donorWallet.GetTransaction(ctx, submitInfo.ID)
	if err != nil {
		t.Error(err)
	}

	if getInfo.ID != submitInfo.ID {
		t.Error("Transaction id mismatch!")
	}

	if getInfo.Destination != destWallet.ProviderID {
		t.Error("Transaction destination mismatch!")
	}

	if !getInfo.Probi.Equals(submitInfo.Probi) {
		t.Error("Transaction probi mismatch!")
	}

	balance, err = destWallet.GetBalance(ctx, true)
	if err != nil {
		t.Error(err)
	}

	if balance.TotalProbi.Equals(decimal.Zero) {
		t.Error("Submit with confirm should result in a balance.")
	}

	// wait for funds to be available
	<-time.After(2 * time.Second)
	txInfo, err := destWallet.Transfer(ctx, altcurrency.BAT, submitInfo.Probi, donorWallet.ProviderID)
	if err != nil {
		t.Log(err)
		t.Log(errors.Unwrap(err))
		t.FailNow()
	}

	if txInfo == nil {
		t.Fatalf("no tx information from transfer!")
	}

	balance, err = destWallet.GetBalance(ctx, true)
	if err != nil {
		t.Error(err)
	}

	if !balance.TotalProbi.Equals(decimal.Zero) {
		t.Error("Transfer should move balance back to donorWallet.")
	}

	if !submitInfo.Probi.Equals(txInfo.Probi) {
		t.Error("Transaction amount should match")
	}

	if len(txInfo.ID) == 0 {
		t.Error("Transaction should have identifier")
	}
}

func TestFingerprintCheck(t *testing.T) {
	var proxy func(*http.Request) (*url.URL, error)
	wrongFingerprint := "IYSLsapSKlkofKfi6M2hmS4gzXbQKGIX/DHBWIgstw3="

	client := &http.Client{
		Timeout: time.Second * 60,
		// remove middleware calls
		Transport: &http.Transport{
			Proxy:          proxy,
			DialTLSContext: pindialer.MakeContextDialer(wrongFingerprint),
		},
	}

	w := requireDonorWallet(t)

	req, err := w.signRegistration("randomlabel")
	if err != nil {
		t.Error(err)
	}

	_, err = client.Do(req)
	// should fail here
	if err == nil {
		t.Error("unable to fail with bad cert")
	}
	assert.Equal(t, errors.Unwrap(err).Error(), "failed to validate certificate chain: the server certificate was not valid")
}

func requireDonorWallet(t *testing.T) *Wallet {
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

	return &Wallet{Info: info, PrivKey: privateKey, PubKey: publicKey}
}

func TestRedactUnneeded(t *testing.T) {
	response := `{"description":"some unneeded content","UKCountry":"foo","TestCountry":"bar","NoMatch": true}`
	result := `{"NoMatch": true}`
	testValue := redactUnneededContent(response)
	assert.Equal(t, result, testValue)
	var dat map[string]interface{}
	if err := json.Unmarshal([]byte(testValue), &dat); err != nil {
		t.Fatal(err)
	}
}

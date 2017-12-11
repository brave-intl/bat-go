// +build integration

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
	jose "github.com/square/go-jose"
	"golang.org/x/crypto/ed25519"
)

var handler http.Handler
var accessToken string
var publicKey ed25519.PublicKey
var privateKey ed25519.PrivateKey

func init() {
	os.Setenv("ENV", "production")

	accessToken = uuid.NewV4().String()
	middleware.TokenList = []string{accessToken}

	var err error
	publicKey, privateKey, err = ed25519.GenerateKey(nil)
	grant.GrantSignatorPublicKeyHex = hex.EncodeToString(publicKey)
	if err != nil {
		log.Fatalln(err)
	}

	handler = chi.ServerBaseContext(setupRouter(setupLogger(context.Background())))
}

func TestPing(t *testing.T) {
	server := httptest.NewServer(handler)
	defer server.Close()
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("Received non-200 response: %d\n", resp.StatusCode)
	}
	expected := "."
	actual, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if expected != string(actual) {
		t.Errorf("Expected the message '%s'\n", expected)
	}
}

func claim(t *testing.T, server *httptest.Server, grantID uuid.UUID, cardID uuid.UUID) error {
	payload := fmt.Sprintf(`{"wallet": {"altcurrency":"BAT", "provider":"uphold", "providerId":"%s"}}`, cardID.String())
	claimURL := fmt.Sprintf("%s/v1/grants/%s", server.URL, grantID.String())

	req, err := http.NewRequest("PUT", claimURL, bytes.NewBuffer([]byte(payload)))
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", "Bearer "+accessToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("Received non-200 response: %d\n", resp.StatusCode)
	}
	return nil
}

func TestClaim(t *testing.T) {
	server := httptest.NewServer(handler)
	defer server.Close()

	grantID := uuid.NewV4()
	cardID := uuid.NewV4()

	err := claim(t, server, grantID, cardID)
	if err != nil {
		t.Fatal(err)
	}
	err = claim(t, server, grantID, uuid.NewV4())
	if err == nil {
		t.Fatal("Expected re-claim of the same grant to a different card to fail")
	}
}

func generateWallet(t *testing.T) *uphold.Wallet {
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
	info.PublicKey = hex.EncodeToString(publicKey)

	newWallet := &uphold.Wallet{info, privateKey, publicKey}
	err = newWallet.Register("bat-go test card")
	if err != nil {
		t.Fatal(err)
	}
	return newWallet
}

func TestRedeem(t *testing.T) {
	server := httptest.NewServer(handler)
	defer server.Close()

	userWallet := generateWallet(t)
	cardID, err := uuid.FromString(userWallet.Info.ProviderID)
	if err != nil {
		log.Fatalln(err)
	}

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: "EdDSA", Key: privateKey}, nil)
	if err != nil {
		log.Fatalln(err)
	}

	maturityDate := time.Now()
	// + 1 week
	expiryDate := maturityDate.AddDate(0, 0, 1)

	grants := grant.CreateGrants(signer, uuid.NewV4(), 1, altcurrency.BAT, 30, maturityDate, expiryDate)
	g, err := grant.FromCompactJWS(grants[0])
	if err != nil {
		log.Fatalln(err)
	}

	grantID := g.GrantID

	err = claim(t, server, grantID, cardID)
	if err != nil {
		t.Fatal(err)
	}

	txn, err := userWallet.PrepareTransaction(altcurrency.BAT, g.Probi, grant.SettlementDestination)
	if err != nil {
		t.Fatal(err)
	}

	var reqPayload grant.RedeemGrantsRequest
	reqPayload.Grants = grants
	reqPayload.WalletInfo = userWallet.Info
	reqPayload.Transaction = txn

	payload, err := json.Marshal(&reqPayload)
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("POST", server.URL+"/v1/grants", bytes.NewBuffer([]byte(payload)))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Authorization", "Bearer "+accessToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("Received non-200 response: %d\n", resp.StatusCode)
	}
}

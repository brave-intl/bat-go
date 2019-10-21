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
	"github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/ed25519"
	jose "gopkg.in/square/go-jose.v2"
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

func claim(t *testing.T, server *httptest.Server, grant grant.Grant, wallet wallet.Info) error {
	payload := fmt.Sprintf(`{
			"wallet": {
				"altcurrency": "BAT", 
				"provider": "uphold", 
				"paymentId": "%s",
				"providerId": "%s",
				"publicKey": "%s"
			},
			"promotionId": "%s"
		}`, wallet.ID, wallet.ProviderID, wallet.PublicKey, grant.PromotionID.String())
	claimURL := fmt.Sprintf("%s/v1/grants/claim", server.URL)

	req, err := http.NewRequest("POST", claimURL, bytes.NewBuffer([]byte(payload)))
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
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			body = []byte("")
		}
		return fmt.Errorf("Received non-200 response: %d, %s\n", resp.StatusCode, body)
	}
	return nil
}

func TestClaim(t *testing.T) {
	server := httptest.NewServer(handler)
	defer server.Close()

	pg, err := promotion.NewPostgres("", false)
	if err != nil {
		t.Fatal(err)
	}

	value := decimal.NewFromFloat(30.0)
	numGrants := 1
	promotion, err := pg.CreatePromotion("ugp", numGrants, value, "")
	if err != nil {
		t.Fatal(err)
	}

	err = pg.ActivatePromotion(promotion)
	if err != nil {
		t.Fatal(err)
	}

	var grant grant.Grant
	grant.GrantID = uuid.NewV4()
	grant.Probi = altcurrency.BAT.ToProbi(value)
	grant.PromotionID = promotion.ID

	var wallet wallet.Info
	wallet.ID = uuid.NewV4().String()
	wallet.ProviderID = uuid.NewV4().String()

	err = claim(t, server, grant, wallet)
	if err != nil {
		t.Fatal(err)
	}

	wallet.ID = uuid.NewV4().String()
	wallet.ProviderID = uuid.NewV4().String()
	err = claim(t, server, grant, wallet)
	if err == nil {
		t.Fatal("Expected re-claim of the same grant to a different card to fail")
	}
}

func generateWallet(t *testing.T) *uphold.Wallet {
	var info wallet.Info
	info.ID = uuid.NewV4().String()
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
	newWallet := &uphold.Wallet{Info: info, PrivKey: privateKey, PubKey: publicKey}
	err = newWallet.Register("bat-go test card")
	if err != nil {
		t.Fatal(err)
	}
	return newWallet
}

func TestRedeem(t *testing.T) {
	server := httptest.NewServer(handler)
	defer server.Close()

	pg, err := promotion.NewPostgres("", false)
	if err != nil {
		t.Fatal(err)
	}

	userWallet := generateWallet(t)

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: "EdDSA", Key: privateKey}, nil)
	if err != nil {
		log.Fatalln(err)
	}

	value := decimal.NewFromFloat(30.0)
	numGrants := 1
	promotion, err := pg.CreatePromotion("ugp", numGrants, value, "")
	if err != nil {
		t.Fatal(err)
	}

	err = pg.ActivatePromotion(promotion)
	if err != nil {
		t.Fatal(err)
	}

	maturityDate := time.Now()
	// + 1 week
	expiryDate := maturityDate.AddDate(0, 0, 1)

	altCurrency := altcurrency.BAT

	grantTemplate := grant.Grant{
		AltCurrency:       &altCurrency,
		Probi:             altcurrency.BAT.ToProbi(value),
		PromotionID:       promotion.ID,
		MaturityTimestamp: maturityDate.Unix(),
		ExpiryTimestamp:   expiryDate.Unix(),
	}

	grants, err := grant.CreateGrants(signer, grantTemplate, 1)
	if err != nil {
		log.Fatalln(err)
	}
	g, err := grant.FromCompactJWS(publicKey, grants[0])
	if err != nil {
		log.Fatalln(err)
	}

	err = claim(t, server, *g, userWallet.Info)
	if err != nil {
		t.Fatal(err)
	}

	txn, err := userWallet.PrepareTransaction(altcurrency.BAT, g.Probi, grant.SettlementDestination, "bat-go:grant-server.TestRedeem")
	if err != nil {
		t.Fatal(err)
	}

	var reqPayload grant.RedeemGrantsRequest
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
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		t.Error(string(bodyBytes))
		t.Fatalf("Received non-200 response: %d\n", resp.StatusCode)
	}
}

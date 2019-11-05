// +build integration

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	log "github.com/sirupsen/logrus"

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
)

var handler http.Handler
var accessToken string
var publicKey ed25519.PublicKey
var privateKey ed25519.PrivateKey

func init() {
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

func claim(t *testing.T, server *httptest.Server, promotionID uuid.UUID, wallet wallet.Info) error {
	payload := fmt.Sprintf(`{
			"wallet": {
				"altcurrency": "BAT", 
				"provider": "uphold", 
				"paymentId": "%s",
				"providerId": "%s",
				"publicKey": "%s"
			},
			"promotionId": "%s"
		}`, wallet.ID, wallet.ProviderID, wallet.PublicKey, promotionID.String())
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

func getPromotions(t *testing.T, server *httptest.Server, wallet wallet.Info) ([]promotion.Promotion, error) {
	type promotionsResp struct {
		Promotions []promotion.Promotion
	}
	promotionsURL := fmt.Sprintf("%s/v1/promotions?legacy=true&paymentId=%s&platform=%s", server.URL, wallet.ID, "osx")
	promotions := []promotion.Promotion{}

	req, err := http.NewRequest("GET", promotionsURL, nil)
	if err != nil {
		return promotions, err
	}

	req.Header.Add("Authorization", "Bearer "+accessToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return promotions, err
	}

	if resp.StatusCode != 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			body = []byte("")
		}
		return promotions, fmt.Errorf("Received non-200 response: %d, %s\n", resp.StatusCode, body)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return promotions, err
	}

	promoResp := promotionsResp{}
	err = json.Unmarshal(body, &promoResp)
	promotions = promoResp.Promotions
	return promotions, err
}

func getActive(t *testing.T, server *httptest.Server, wallet wallet.Info) ([]grant.Grant, error) {
	type grantsResp struct {
		Grants []grant.Grant
	}
	grantsURL := fmt.Sprintf("%s/v1/grants/active?paymentId=%s", server.URL, wallet.ID)
	grants := []grant.Grant{}

	req, err := http.NewRequest("GET", grantsURL, nil)
	if err != nil {
		return grants, err
	}

	req.Header.Add("Authorization", "Bearer "+accessToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return grants, err
	}

	if resp.StatusCode != 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			body = []byte("")
		}
		return grants, fmt.Errorf("Received non-200 response: %d, %s\n", resp.StatusCode, body)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return grants, err
	}

	r := grantsResp{}
	err = json.Unmarshal(body, &r)
	grants = r.Grants
	return grants, err
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
	promotion, err := pg.CreatePromotion("ugp", numGrants, value, "android")
	if err != nil {
		t.Fatal(err)
	}

	err = pg.ActivatePromotion(promotion)
	if err != nil {
		t.Fatal(err)
	}

	var wallet wallet.Info
	wallet.ID = uuid.NewV4().String()
	wallet.ProviderID = uuid.NewV4().String()

	err = claim(t, server, promotion.ID, wallet)
	if err != nil {
		t.Fatal(err)
	}

	grants, err := getActive(t, server, wallet)
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 1 {
		t.Fatal("Expected one active grant")
	}
	if grants[0].Type != "android" {
		t.Fatal("Expected one active android grant")
	}
	if !grants[0].Probi.Equals(altcurrency.BAT.ToProbi(value)) {
		t.Fatal("Expected one active android grant worth 30 BAT")
	}

	wallet.ID = uuid.NewV4().String()
	wallet.ProviderID = uuid.NewV4().String()
	err = claim(t, server, promotion.ID, wallet)
	if err == nil {
		t.Fatal("Expected re-claim of unavailable promotion to a different card to fail")
	}

	value = decimal.NewFromFloat(35.0)
	numGrants = 2
	promotion, err = pg.CreatePromotion("ugp", numGrants, value, "desktop")
	if err != nil {
		t.Fatal(err)
	}

	err = pg.ActivatePromotion(promotion)
	if err != nil {
		t.Fatal(err)
	}

	err = claim(t, server, promotion.ID, wallet)
	if err != nil {
		t.Fatal(err)
	}

	grants, err = getActive(t, server, wallet)
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 1 {
		t.Fatal("Expected one active grant")
	}
	if grants[0].Type != "ugp" {
		t.Fatal("Expected one active ugp grant")
	}
	if !grants[0].Probi.Equals(altcurrency.BAT.ToProbi(value)) {
		t.Fatal("Expected one active ugp grant worth 35 BAT")
	}

	err = claim(t, server, promotion.ID, wallet)
	if err == nil {
		t.Fatal("Expected re-claim of the same grant to fail despite available grants")
	}

	wallet.ID = uuid.NewV4().String()
	wallet.ProviderID = uuid.NewV4().String()
	err = claim(t, server, promotion.ID, wallet)
	if err != nil {
		t.Fatal("Expected re-claim of the same promotion with available to a different card to succeed")
	}

	wallet.ID = uuid.NewV4().String()
	wallet.ProviderID = uuid.NewV4().String()
	err = claim(t, server, promotion.ID, wallet)
	if err == nil {
		t.Fatal("Expected new claim of the same promotion to fail as there are no more grants")
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

	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions"}
	for _, table := range tables {
		_, err = pg.DB.Exec("delete from " + table)
		if err != nil {
			t.Fatal(err)
		}
	}

	userWallet := generateWallet(t)

	value := decimal.NewFromFloat(10.0)
	numGrants := 1
	promotion, err := pg.CreatePromotion("ugp", numGrants, value, "")
	if err != nil {
		t.Fatal(err)
	}

	err = pg.ActivatePromotion(promotion)
	if err != nil {
		t.Fatal(err)
	}

	err = claim(t, server, promotion.ID, userWallet.Info)
	if err != nil {
		t.Fatal(err)
	}

	value = decimal.NewFromFloat(10.0)
	numGrants = 1
	promotion, err = pg.CreatePromotion("ugp", numGrants, value, "")
	if err != nil {
		t.Fatal(err)
	}

	err = pg.ActivatePromotion(promotion)
	if err != nil {
		t.Fatal(err)
	}

	promotions, err := getPromotions(t, server, userWallet.Info)
	if err != nil {
		t.Fatal(err)
	}
	if len(promotions) != 1 {
		t.Fatal("with two active promotions and one claimed, exactly one promo should be advertised")
	}
	if promotions[0].ID != promotion.ID {
		t.Fatal("promotion id did not match!")
	}

	err = claim(t, server, promotion.ID, userWallet.Info)
	if err != nil {
		t.Fatal(err)
	}

	promotions, err = getPromotions(t, server, userWallet.Info)
	if err != nil {
		t.Fatal(err)
	}
	if len(promotions) != 0 {
		t.Fatal("with two active promotions and two claimed, no promos should be advertised")
	}

	totalBAT := altcurrency.BAT.ToProbi(decimal.NewFromFloat(20.0))
	txBAT := altcurrency.BAT.ToProbi(decimal.NewFromFloat(10.0))

	grants, err := getActive(t, server, userWallet.Info)
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 2 {
		t.Fatal("Expected two active grants")
	}
	if !grants[0].Probi.Add(grants[1].Probi).Equals(totalBAT) {
		t.Fatal("Expected two active android grant worth 20 BAT total")
	}

	txn, err := userWallet.PrepareTransaction(altcurrency.BAT, txBAT, grant.SettlementDestination, "bat-go:grant-server.TestRedeem")
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

	grants, err = getActive(t, server, userWallet.Info)
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 1 {
		t.Fatal("Expected one active grants")
	}

	value = decimal.NewFromFloat(10.0)
	numGrants = 1
	promotion, err = pg.CreatePromotion("ugp", numGrants, value, "")
	if err != nil {
		t.Fatal(err)
	}

	err = pg.ActivatePromotion(promotion)
	if err != nil {
		t.Fatal(err)
	}

	err = claim(t, server, promotion.ID, userWallet.Info)
	if err != nil {
		t.Fatal(err)
	}

	txn, err = userWallet.PrepareTransaction(altcurrency.BAT, totalBAT, grant.SettlementDestination, "bat-go:grant-server.TestRedeem")
	if err != nil {
		t.Fatal(err)
	}

	reqPayload.WalletInfo = userWallet.Info
	reqPayload.Transaction = txn

	payload, err = json.Marshal(&reqPayload)
	if err != nil {
		t.Fatal(err)
	}

	req, err = http.NewRequest("POST", server.URL+"/v1/grants", bytes.NewBuffer([]byte(payload)))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Authorization", "Bearer "+accessToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 200 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		t.Error(string(bodyBytes))
		t.Fatalf("Received non-200 response: %d\n", resp.StatusCode)
	}

	grants, err = getActive(t, server, userWallet.Info)
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 0 {
		t.Fatal("Expected zero active grants")
	}
}

func TestDrain(t *testing.T) {
	server := httptest.NewServer(handler)
	defer server.Close()

	pg, err := promotion.NewPostgres("", false)
	if err != nil {
		t.Fatal(err)
	}

	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions"}
	for _, table := range tables {
		_, err = pg.DB.Exec("delete from " + table)
		if err != nil {
			t.Fatal(err)
		}
	}

	userWallet := generateWallet(t)

	value := decimal.NewFromFloat(10.0)
	numGrants := 1
	promotion, err := pg.CreatePromotion("ugp", numGrants, value, "")
	if err != nil {
		t.Fatal(err)
	}

	err = pg.ActivatePromotion(promotion)
	if err != nil {
		t.Fatal(err)
	}

	err = claim(t, server, promotion.ID, userWallet.Info)
	if err != nil {
		t.Fatal(err)
	}

	value = decimal.NewFromFloat(10.0)
	numGrants = 1
	promotion, err = pg.CreatePromotion("ugp", numGrants, value, "")
	if err != nil {
		t.Fatal(err)
	}

	err = pg.ActivatePromotion(promotion)
	if err != nil {
		t.Fatal(err)
	}

	err = claim(t, server, promotion.ID, userWallet.Info)
	if err != nil {
		t.Fatal(err)
	}

	totalBAT := altcurrency.BAT.ToProbi(decimal.NewFromFloat(20.0))

	grants, err := getActive(t, server, userWallet.Info)
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 2 {
		t.Fatal("Expected two active grants")
	}
	if !grants[0].Probi.Add(grants[1].Probi).Equals(totalBAT) {
		t.Fatal("Expected two active android grant worth 20 BAT total")
	}

	var reqPayload grant.DrainGrantsRequest
	reqPayload.WalletInfo = userWallet.Info
	anonymousAddress, err := uuid.FromString(userWallet.Info.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	reqPayload.AnonymousAddress = anonymousAddress

	payload, err := json.Marshal(&reqPayload)
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("POST", server.URL+"/v1/grants/drain", bytes.NewBuffer([]byte(payload)))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Authorization", "Bearer "+accessToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Error(string(bodyBytes))
		t.Fatalf("Received non-200 response: %d\n", resp.StatusCode)
	}

	var respPayload grant.DrainGrantsResponse
	json.Unmarshal(bodyBytes, &respPayload)
	if err != nil {
		t.Fatal(err)
	}

	if !respPayload.GrantTotal.Equals(totalBAT) {
		t.Fatal("Expected redeemed grants to equal 20 BAT total")
	}

	grants, err = getActive(t, server, userWallet.Info)
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 0 {
		t.Fatal("Expected zero active grants")
	}

	_, err = userWallet.Transfer(altcurrency.BAT, totalBAT, grant.SettlementDestination)
	if err != nil {
		t.Log(err)
	}

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	bodyBytes, _ = ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("Received non-200 response: %d\n", resp.StatusCode)
	}

	json.Unmarshal(bodyBytes, &respPayload)
	if err != nil {
		t.Fatal(err)
	}

	if !respPayload.GrantTotal.Equals(decimal.Zero) {
		t.Fatal("Expected redeemed grants to equal 0 BAT")
	}
}

// This is to try to test a very stubborn validation issue with promotionID
func TestClaimValidation(t *testing.T) {
	server := httptest.NewServer(handler)
	defer server.Close()

	pg, err := promotion.NewPostgres("", false)
	if err != nil {
		t.Fatal(err)
	}

	var w wallet.Info
	w.ID = uuid.NewV4().String()
	w.ProviderID = uuid.NewV4().String()

	err = claim(t, server, uuid.Nil, w)
	if err == nil {
		t.Fatal("Must fail if no promotion id is passed")
	}

	for i := 0; i < 100; i++ {
		value := decimal.NewFromFloat(30.0)
		numGrants := 1
		promotion, err := pg.CreatePromotion("ugp", numGrants, value, "android")
		if err != nil {
			t.Fatal(err)
		}

		err = pg.ActivatePromotion(promotion)
		if err != nil {
			t.Fatal(err)
		}

		var wallet wallet.Info
		wallet.ID = uuid.NewV4().String()
		wallet.ProviderID = uuid.NewV4().String()

		err = claim(t, server, promotion.ID, wallet)
		if err != nil {
			t.Error(promotion.ID)
			t.Fatal(err)
		}
	}
}

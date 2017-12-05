package uphold

import (
	"bytes"
	"crypto"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/digest"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/pindialer"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/ed25519"
)

type UpholdWallet struct {
	wallet.WalletInfo
	PrivKey ed25519.PrivateKey
	PubKey  httpsignature.Ed25519PubKey
}

var (
	accessToken   = os.Getenv("UPHOLD_ACCESS_TOKEN")
	environment   = os.Getenv("UPHOLD_ENVIRONMENT")
	upholdApiBase = map[string]string{
		"":        "https://api-sandbox.uphold.com", // os.Getenv() will return empty string if not set
		"sandbox": "https://api-sandbox.uphold.com",
		"prod":    "https://api.uphold.com",
	}[environment]

	upholdCertFingerprint = "YM2Dejq4VOK/7CorxWBIcHnhKlHzvgFgrLYchGroakc="
	client                = &http.Client{
		Timeout: time.Second * 10,
		Transport: middleware.InstrumentRoundTripper(
			&http.Transport{
				DialTLS: pindialer.MakeDialer(upholdCertFingerprint),
			}, "uphold"),
	}
)

// TODO add context?

func New(info wallet.WalletInfo, privKey ed25519.PrivateKey, pubKey httpsignature.Ed25519PubKey) (*UpholdWallet, error) {
	if info.Provider != "uphold" {
		return nil, errors.New("The wallet provider must be uphold")
	}
	if len(info.ProviderId) > 0 {
		if !govalidator.IsUUIDv4(info.ProviderId) {
			return nil, errors.New("An uphold cardId (the providerId) must be a UUIDv4")
		}
	} else {
		return nil, errors.New("Generation of new uphold wallet is not yet implemented")
	}
	if !info.AltCurrency.IsValid() {
		return nil, errors.New("A wallet must have a valid altcurrency")
	}
	return &UpholdWallet{info, privKey, pubKey}, nil
}

func FromWalletInfo(info wallet.WalletInfo) (*UpholdWallet, error) {
	var publicKey httpsignature.Ed25519PubKey
	if len(info.PublicKey) > 0 {
		publicKey, _ = hex.DecodeString(info.PublicKey)
	}
	return New(info, ed25519.PrivateKey{}, publicKey)
}

func newRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, upholdApiBase+path, body)
	if err == nil {
		req.Header.Add("Authorization", "Bearer "+accessToken)
	}
	return req, err
}

func submit(req *http.Request) (*http.Response, error) {
	req.Header.Add("content-type", "application/json")

	// FIXME dump request on debug loglevel
	//dump, _ := httputil.DumpRequestOut(req, true)
	//fmt.Println(string(dump))

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		body, _ := ioutil.ReadAll(resp.Body)
		var uhErr UpholdError
		if json.Unmarshal(body, &uhErr) != nil {
			return nil, errors.New(fmt.Sprintf("Error %d, %s", resp.StatusCode, body))
		} else {
			return nil, uhErr
		}
	}
	return resp, nil
}

type CardSettings struct {
	Protected bool `json:"protected,omitempty"`
}

type CardDetails struct {
	Currency         altcurrency.AltCurrency `json:"currency"`
	Balance          decimal.Decimal         `json:"balance"`
	AvailableBalance decimal.Decimal         `json:"available"`
	Settings         CardSettings            `json:"settings"`
}

func (w *UpholdWallet) GetCardDetails() (*CardDetails, error) {
	req, err := newRequest("GET", "/v0/me/cards/"+w.ProviderId, nil)
	if err != nil {
		return nil, err
	}
	resp, err := submit(req)
	if err != nil {
		return nil, err
	}

	var details CardDetails
	err = json.NewDecoder(resp.Body).Decode(&details)
	if err != nil {
		return nil, err
	}
	return &details, err
}

func (w *UpholdWallet) UpdatePublicKey() error {
	return nil
}

func (w *UpholdWallet) GetWalletInfo() wallet.WalletInfo {
	return w.WalletInfo
}

type Denomination struct {
	Amount   decimal.Decimal          `json:"amount"`
	Currency *altcurrency.AltCurrency `json:"currency"`
}

type TransactionRequest struct {
	Denomination Denomination `json:"denomination"`
	Destination  string       `json:"destination"`
}

func (w *UpholdWallet) signTransfer(altcurrency altcurrency.AltCurrency, probi decimal.Decimal, destination string) (*http.Request, error) {
	transferReq := TransactionRequest{Denomination{altcurrency.FromProbi(probi), &altcurrency}, destination}
	unsignedTransaction, err := json.Marshal(&transferReq)
	if err != nil {
		return nil, err
	}

	req, err := newRequest("POST", "/v0/me/cards/"+w.ProviderId+"/transactions?commit=true", bytes.NewBuffer(unsignedTransaction))
	if err != nil {
		return nil, err
	}

	var s httpsignature.Signature
	s.Algorithm = httpsignature.ED25519
	s.KeyId = "primary"
	s.Headers = []string{"digest"}

	// FIXME digest calc should move to httpsignature lib
	var d digest.Instance
	d.Hash = crypto.SHA256
	d.Update(unsignedTransaction)
	req.Header.Add("Digest", d.String())

	err = s.Sign(w.PrivKey, crypto.Hash(0), req)
	return req, err
}

func (w *UpholdWallet) EncapsulateTransfer(altcurrency altcurrency.AltCurrency, probi decimal.Decimal, destination string) (*httpsignature.HttpSignedRequest, error) {
	req, err := w.signTransfer(altcurrency, probi, destination)
	if err != nil {
		return nil, err
	}
	return httpsignature.Encapsulate(req)
}

func (w *UpholdWallet) Transfer(altcurrency altcurrency.AltCurrency, probi decimal.Decimal, destination string) (*wallet.TransactionInfo, error) {
	req, err := w.signTransfer(altcurrency, probi, destination)
	if err != nil {
		return nil, err
	}
	_, err = submit(req)
	if err != nil {
		return nil, err
	}

	var txInfo wallet.TransactionInfo
	txInfo.Probi = probi
	{
		tmp := altcurrency
		txInfo.AltCurrency = &tmp
	}
	txInfo.Destination = destination

	return &txInfo, nil
}

func (w *UpholdWallet) DecodeTransaction(transactionB64 string) (*TransactionRequest, error) {
	b, err := base64.StdEncoding.DecodeString(transactionB64)
	if err != nil {
		return nil, err
	}

	var signedTx httpsignature.HttpSignedRequest
	err = json.Unmarshal(b, &signedTx)
	if err != nil {
		return nil, err
	}

	digestHeader, exists := signedTx.Headers["digest"]
	if !exists {
		return nil, errors.New("A transaction signature must cover the request body via digest")
	}

	var digest digest.Instance
	err = digest.UnmarshalText([]byte(digestHeader))
	if err != nil {
		return nil, err
	}

	if !digest.Verify([]byte(signedTx.Body)) {
		return nil, errors.New("The digest header does not match the included body")
	}

	sig, req, err := signedTx.Extract()
	if err != nil {
		return nil, err
	}

	exists = false
	for _, header := range sig.Headers {
		if header == "digest" {
			exists = true
		}
	}
	if !exists {
		return nil, errors.New("A transaction signature must cover the request body via digest")
	}

	valid, err := sig.Verify(w.PubKey, crypto.Hash(0), req)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, errors.New("The signature is invalid")
	}

	var transaction TransactionRequest
	err = json.Unmarshal([]byte(signedTx.Body), &transaction)
	if err != nil {
		return nil, err
	}

	if !govalidator.IsEmail(transaction.Destination) {
		if !govalidator.IsUUIDv4(transaction.Destination) {
			if !utils.IsBTCAddress(transaction.Destination) {
				if !utils.IsETHAddress(transaction.Destination) {
					return nil, errors.New(fmt.Sprintf("%s is not a valid destination", transaction.Destination))
				}
			}
		}
	}

	// NOTE we are effectively stuck using two different JSON parsers on the same data as our parser
	// is different than Uphold's. this has the unfortunate effect of opening us to attacks
	// that exploit differences between parsers. to mitigate this we will be extremely strict
	// in parsing, requiring that the remarshalled struct is equivalent. this means the order
	// of fields must be identical as well as numeric serialization. for encoding/json, note
	// that struct keys are serialized in the order they are defined

	remarshalledBody, err := json.Marshal(&transaction)
	if err != nil {
		return nil, err
	}

	if string(remarshalledBody) != signedTx.Body {
		return nil, errors.New("The remarshalled body must be identical")
	}

	return &transaction, nil
}

// NOTE VerifyTransaction guards against transactions that seek to exploit parser differences
// such as including additional fields that are not understood by this implementation but may
// be understood by the upstream wallet provider. See DecodeTransaction for details.
func (w *UpholdWallet) VerifyTransaction(transactionB64 string) (*wallet.TransactionInfo, error) {
	transaction, err := w.DecodeTransaction(transactionB64)
	if err != nil {
		return nil, err
	}
	var info wallet.TransactionInfo
	info.Probi = transaction.Denomination.Currency.ToProbi(transaction.Denomination.Amount)
	{
		tmp := *transaction.Denomination.Currency
		info.AltCurrency = &tmp
	}
	info.Destination = transaction.Destination

	return &info, err
}

type UpholdTransactionResponseDestination struct {
	Type   string `json:"type"`
	CardID string `json:"CardId"`
}

type UpholdTransactionResponse struct {
	Status       string                               `json:"status"`
	ID           string                               `json:"id"`
	Denomination Denomination                         `json:"denomination"`
	Destination  UpholdTransactionResponseDestination `json:"destination"`
}

func (w *UpholdWallet) SubmitTransaction(transactionB64 string, confirm bool) (*wallet.TransactionInfo, error) {
	info, err := w.VerifyTransaction(transactionB64)
	if err != nil {
		return nil, err
	}

	b, _ := base64.StdEncoding.DecodeString(transactionB64)
	var signedTx httpsignature.HttpSignedRequest
	json.Unmarshal(b, &signedTx)

	var headers http.Header
	var body io.ReadCloser
	{
		_, req, err := signedTx.Extract()
		if err != nil {
			return nil, err
		}
		headers = req.Header
		body = req.Body
	}

	url := "/v0/me/cards/" + w.ProviderId + "/transactions"
	if confirm {
		url = url + "?commit=true"
	}
	req, err := newRequest("POST", url, nil)
	if err != nil {
		return nil, err
	}

	// Copy headers added from newRequest
	for k, _ := range req.Header {
		headers.Set(k, req.Header.Get(k))
	}
	req.Header = headers
	req.Body = body

	resp, err := submit(req)
	if err != nil {
		return nil, err
	}

	respBody, _ := ioutil.ReadAll(resp.Body)
	var uhResp UpholdTransactionResponse
	err = json.Unmarshal(respBody, &uhResp)
	if err != nil {
		return nil, err
	}

	info.Fee = decimal.Zero
	info.Status = uhResp.Status
	info.ID = uhResp.ID

	return info, nil
}

func (w *UpholdWallet) ConfirmTransaction(id string) (*wallet.TransactionInfo, error) {
	req, err := newRequest("POST", "/v0/me/cards/"+w.ProviderId+"/transactions/"+id+"/commit", nil)
	if err != nil {
		return nil, err
	}
	resp, err := submit(req)
	if err != nil {
		return nil, err
	}

	respBody, _ := ioutil.ReadAll(resp.Body)
	var uhResp UpholdTransactionResponse
	err = json.Unmarshal(respBody, &uhResp)
	if err != nil {
		return nil, err
	}

	if uhResp.Destination.Type != "card" {
		panic("Confirming a non-card transaction is not supported!!!")
	}

	var txInfo wallet.TransactionInfo
	txInfo.Probi = uhResp.Denomination.Currency.ToProbi(uhResp.Denomination.Amount)
	{
		tmp := *uhResp.Denomination.Currency
		txInfo.AltCurrency = &tmp
	}
	txInfo.Destination = uhResp.Destination.CardID
	txInfo.Fee = decimal.Zero
	txInfo.Status = uhResp.Status
	txInfo.ID = uhResp.ID

	return &txInfo, nil
}

func (w *UpholdWallet) GetBalance(refresh bool) (*wallet.Balance, error) {
	if !refresh {
		return w.LastBalance, nil
	}

	var balance wallet.Balance

	details, err := w.GetCardDetails()
	if err != nil {
		return nil, err
	}

	if details.Currency != *w.AltCurrency {
		return nil, errors.New("Returned currency did not match wallet altcurrency")
	}

	balance.TotalProbi = details.Currency.ToProbi(details.Balance)
	balance.SpendableProbi = details.Currency.ToProbi(details.AvailableBalance)
	balance.ConfirmedProbi = balance.SpendableProbi
	balance.UnconfirmedProbi = balance.TotalProbi.Sub(balance.SpendableProbi)
	w.LastBalance = &balance

	return &balance, nil
}

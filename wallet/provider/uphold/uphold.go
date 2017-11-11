package uphold

import (
	"bytes"
	"crypto"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/digest"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/ed25519"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"
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
	client = &http.Client{
		Timeout:   time.Second * 10,
		Transport: middleware.InstrumentedRoundTripper("uphold"),
	}
)

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
	return New(info, ed25519.PrivateKey{}, httpsignature.Ed25519PubKey{})
}

func newRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, upholdApiBase+path, body)
	if err == nil {
		req.Header.Add("Authorization", "Bearer "+accessToken)
	}
	return req, err
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
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, errors.New(fmt.Sprintf("Error %d, %s", resp.StatusCode, body))
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
	Amount   decimal.Decimal         `json:"amount"`
	Currency altcurrency.AltCurrency `json:"currency"`
}

type TransactionRequest struct {
	Denomination Denomination `json:"denomination"`
	Destination  string       `json:"destination"`
}

func (w *UpholdWallet) Transfer(altcurrency altcurrency.AltCurrency, probi decimal.Decimal, destination string) (*wallet.TransactionInfo, error) {
	// FIXME
	return nil, nil
}

// An encapsulated HTTP signature transaction
type HttpSignedTransactionRequest struct {
	Headers map[string]string `json:"headers"` //TODO add verfier to ensure only lower-cased keys
	Body    string            `json:"octets"`
}

func (sr *HttpSignedTransactionRequest) Extract() (*httpsignature.Signature, *http.Request, error) {
	var s httpsignature.Signature
	err := s.UnmarshalText([]byte(sr.Headers["signature"]))
	if err != nil {
		return nil, nil, err
	}

	var r http.Request
	r.Header = http.Header{}
	for k, v := range sr.Headers {
		if k == httpsignature.RequestTarget {
			// TODO
			return nil, nil, errors.New(fmt.Sprintf("%s pseudo-header not implemented", httpsignature.RequestTarget))
		} else {
			r.Header.Set(k, v)
		}
	}
	return &s, &r, nil
}

func (w *UpholdWallet) DecodeTransaction(transactionB64 string) (*TransactionRequest, error) {
	b, err := base64.StdEncoding.DecodeString(transactionB64)
	if err != nil {
		return nil, err
	}

	var signedTx HttpSignedTransactionRequest
	err = json.Unmarshal(b, &signedTx)
	if err != nil {
		return nil, err
	}

	digestHeader, exists := signedTx.Headers["digest"]
	if !exists {
		return nil, errors.New("A transaction signature must cover the request body via digest")
	}

	var digest digest.DigestInstance
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

	// FIXME could be extra fields in body :/ any way to check?

	if !govalidator.IsEmail(transaction.Destination) {
		if !govalidator.IsUUIDv4(transaction.Destination) {
			if !utils.IsBTCAddress(transaction.Destination) {
				if !utils.IsETHAddress(transaction.Destination) {
					return nil, errors.New(fmt.Sprintf("%s is not a valid destination", transaction.Destination))
				}
			}
		}
	}

	return &transaction, nil
}

func (w *UpholdWallet) VerifyTransaction(transactionB64 string) (*wallet.TransactionInfo, error) {
	transaction, err := w.DecodeTransaction(transactionB64)
	if err != nil {
		return nil, err
	}
	var info wallet.TransactionInfo
	info.Probi = transaction.Denomination.Currency.ToProbi(transaction.Denomination.Amount)
	info.AltCurrency = transaction.Denomination.Currency
	info.Destination = transaction.Destination

	return &info, err
}

func (w *UpholdWallet) SubmitTransaction(transactionB64 string) (*wallet.TransactionInfo, error) {
	info, err := w.VerifyTransaction(transactionB64)
	if err != nil {
		return nil, err
	}

	b, _ := base64.StdEncoding.DecodeString(transactionB64)
	var signedTx HttpSignedTransactionRequest
	json.Unmarshal(b, &signedTx)

	req, err := newRequest("POST", "/v0/me/cards/"+w.ProviderId+"/transactions?commit=true", bytes.NewBufferString(signedTx.Body))
	if err != nil {
		return nil, err
	}

	for k, v := range signedTx.Headers {
		req.Header.Add(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		body, _ := ioutil.ReadAll(resp.Body)
		var uhErr UpholdError
		if json.Unmarshal(b, &uhErr) != nil {
			return nil, errors.New(fmt.Sprintf("Error %d, %s", resp.StatusCode, body))
		} else {
			return nil, uhErr
		}
	}

	return info, nil
}

func (w *UpholdWallet) GetBalance(refresh bool) (*wallet.Balance, error) {
	if !refresh {
		return &w.LastBalance, nil
	}

	var balance wallet.Balance

	details, err := w.GetCardDetails()
	if err != nil {
		return nil, err
	}

	if details.Currency != w.AltCurrency {
		return nil, errors.New("Returned currency did not match wallet altcurrency")
	}

	balance.TotalProbi = details.Currency.ToProbi(details.Balance)
	balance.SpendableProbi = details.Currency.ToProbi(details.AvailableBalance)
	balance.ConfirmedProbi = balance.SpendableProbi
	balance.UnconfirmedProbi = balance.TotalProbi.Sub(balance.SpendableProbi)
	w.LastBalance = balance

	return &balance, nil
}

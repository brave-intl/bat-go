package uphold

import (
	"bytes"
	"context"
	"crypto"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/digest"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/pindialer"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/brave-intl/bat-go/utils/validators"
	"github.com/brave-intl/bat-go/utils/wallet"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/rs/zerolog"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/ed25519"
)

// Wallet a wallet information using Uphold as the provider
// A wallet corresponds to a single Uphold "card"
type Wallet struct {
	walletutils.Info
	logger  *zerolog.Logger
	PrivKey crypto.Signer
	PubKey  httpsignature.Verifier
}

const (
	dateFormat              = "2006-01-02T15:04:05.000Z"
	batchSize               = 50
	listTransactionsRetries = 5
)

const (
	sandboxFingerprint = "IYSLsapSKlkofKfi6M2hmS4gzXbQKGIX/DHBWIgstw4="
	prodFingerprint    = "IYSLsapSKlkofKfi6M2hmS4gzXbQKGIX/DHBWIgstw4="
)

var (
	// filter out authorization tokens from logs
	authLogFilter = regexp.MustCompile(`Authorization: .+\n`)

	// SettlementDestination is the address of the settlement wallet
	SettlementDestination = os.Getenv("BAT_SETTLEMENT_ADDRESS")

	// AnonCardSettlementAddress is the address of the settlement wallet
	AnonCardSettlementAddress = os.Getenv("ANON_CARD_SETTLEMENT_ADDRESS")
	// UpholdSettlementAddress is the address of the settlement wallet
	UpholdSettlementAddress = os.Getenv("UPHOLD_SETTLEMENT_ADDRESS")

	grantWalletCardID     = os.Getenv("GRANT_WALLET_CARD_ID")
	grantWalletPrivateKey = os.Getenv("GRANT_WALLET_PRIVATE_KEY")
	grantWalletPublicKey  = os.Getenv("GRANT_WALLET_PUBLIC_KEY")

	accessToken   = os.Getenv("UPHOLD_ACCESS_TOKEN")
	environment   = os.Getenv("UPHOLD_ENVIRONMENT")
	upholdProxy   = os.Getenv("UPHOLD_HTTP_PROXY")
	upholdAPIBase = map[string]string{
		"":        "https://api-sandbox.uphold.com", // os.Getenv() will return empty string if not set
		"sandbox": "https://api-sandbox.uphold.com",
		"prod":    "https://api.uphold.com",
	}[environment]
	upholdCertFingerprint = map[string]string{
		"":        sandboxFingerprint, // os.Getenv() will return empty string if not set
		"sandbox": sandboxFingerprint,
		"prod":    prodFingerprint,
	}[environment]
	client *http.Client
)

func init() {

	// Default back to BAT_SETTLEMENT_ADDRESS
	if AnonCardSettlementAddress == "" {
		AnonCardSettlementAddress = SettlementDestination
	}
	if UpholdSettlementAddress == "" {
		UpholdSettlementAddress = SettlementDestination
	}

	var proxy func(*http.Request) (*url.URL, error)
	if len(upholdProxy) > 0 {
		proxyURL, err := url.Parse(upholdProxy)
		if err != nil {
			panic("UPHOLD_HTTP_PROXY is not a valid proxy URL")
		}
		proxy = http.ProxyURL(proxyURL)
	} else {
		proxy = nil
	}
	client = &http.Client{
		Timeout: time.Second * 60,
		Transport: middleware.InstrumentRoundTripper(
			&http.Transport{
				Proxy:   proxy,
				DialTLS: pindialer.MakeDialer(upholdCertFingerprint),
			}, "uphold"),
	}
}

// New returns an uphold wallet constructed using the provided parameters
// NOTE that it does not register a wallet with Uphold if it does not already exist
func New(ctx context.Context, info walletutils.Info, privKey crypto.Signer, pubKey httpsignature.Verifier) (*Wallet, error) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	if info.Provider != "uphold" {
		return nil, errors.New("The wallet provider must be uphold")
	}
	if len(info.ProviderID) > 0 {
		if !validators.IsUUID(info.ProviderID) {
			return nil, errors.New("An uphold cardId (the providerId) must be a UUIDv4")
		}
	} else {
		return nil, errors.New("Generation of new uphold wallet is not yet implemented")
	}
	if !info.AltCurrency.IsValid() {
		return nil, errors.New("A wallet must have a valid altcurrency")
	}
	return &Wallet{logger: logger, Info: info, PrivKey: privKey, PubKey: pubKey}, nil
}

// FromWalletInfo returns an uphold wallet matching the provided wallet info
func FromWalletInfo(ctx context.Context, info walletutils.Info) (*Wallet, error) {
	var publicKey httpsignature.Ed25519PubKey
	if len(info.PublicKey) > 0 {
		var err error
		publicKey, err = hex.DecodeString(info.PublicKey)
		if err != nil {
			return nil, err
		}
	}
	return New(ctx, info, ed25519.PrivateKey{}, publicKey)
}

func newRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, upholdAPIBase+path, body)
	if err == nil {
		req.Header.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(accessToken+":X-OAuth-Basic")))
	}
	return req, err
}

func submit(logger *zerolog.Logger, req *http.Request) ([]byte, *http.Response, error) {
	req.Header.Add("content-type", "application/json")

	dump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		panic(err)
	}
	dump = authLogFilter.ReplaceAll(dump, []byte("Authorization: Basic <token>\n"))

	if logger != nil {
		logger.Debug().
			Str("path", "github.com/brave-intl/bat-go/wallet/provider/uphold").
			Str("type", "http.Request").
			Msg(string(dump))
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, resp, err
	}

	headers := map[string][]string(resp.Header)
	jsonHeaders, err := json.MarshalIndent(headers, "", "    ")
	if err != nil {
		return nil, resp, err
	}

	body, err := requestutils.Read(resp.Body)
	if err != nil {
		return nil, resp, err
	}

	if logger != nil {
		logger.Debug().
			Str("path", "github.com/brave-intl/bat-go/wallet/provider/uphold").
			Str("type", "http.Response").
			Int("status", resp.StatusCode).
			Str("headers", string(jsonHeaders)).
			Msg(string(dump))
	}

	if resp.StatusCode/100 != 2 {
		var uhErr upholdError
		if json.Unmarshal(body, &uhErr) != nil {
			return nil, resp, fmt.Errorf("Error %d, %s", resp.StatusCode, body)
		}
		return nil, resp, uhErr
	}
	return body, resp, nil
}

type createCardRequest struct {
	Label       string                   `json:"label"`
	AltCurrency *altcurrency.AltCurrency `json:"currency"`
	PublicKey   string                   `json:"publicKey"`
}

// IsUserKYC - is this user a "member"
func (w *Wallet) IsUserKYC(ctx context.Context) (bool, error) {
	// get logger
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		_, logger = logging.SetupLogger(ctx)
	}

	// in order to get the isMember status of the wallet, we need to start
	// a transaction of 0 BAT to the wallet "w" from "grant_wallet" but never commit

	gwPublicKey, err := hex.DecodeString(grantWalletPublicKey)
	if err != nil {
		logger.Error().Err(err).Msg("invalid system public key")
		return false, fmt.Errorf("invalid system public key: %w", err)
	}
	gwPrivateKey, err := hex.DecodeString(grantWalletPrivateKey)
	if err != nil {
		logger.Error().Err(err).Msg("invalid system private key")
		return false, fmt.Errorf("invalid system private key: %w", err)
	}

	grantWallet := Wallet{
		Info: walletutils.Info{
			ProviderID: grantWalletCardID,
			Provider:   "uphold",
			PublicKey:  grantWalletPublicKey,
		},
		PrivKey: ed25519.PrivateKey([]byte(gwPrivateKey)),
		PubKey:  httpsignature.Ed25519PubKey([]byte(gwPublicKey)),
	}

	// prepare a transaction by creating a payload
	transactionB64, err := grantWallet.PrepareTransaction(altcurrency.BAT, decimal.New(0, 1).String(), w.Info.ProviderID, "")
	if err != nil {
		logger.Error().Err(err).Msg("failed to prepare transaction")
		return false, fmt.Errorf("failed to prepare transaction: %w", err)
	}

	// submit the transaction the payload
	uhResp, err := grantWallet.SubmitTransaction(transactionB64, false)
	if err != nil {
		logger.Error().Err(err).Msg("failed to submit transaction")
		return false, fmt.Errorf("failed to submit transaction: %w", err)
	}

	return uhResp.KYC, nil
}

// sign registration for this wallet with Uphold with label
func (w *Wallet) signRegistration(label string) (*http.Request, error) {
	reqPayload := createCardRequest{Label: label, AltCurrency: w.Info.AltCurrency, PublicKey: w.PubKey.String()}
	payload, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	req, err := newRequest("POST", "/v0/me/cards", bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}

	var s httpsignature.Signature
	s.Algorithm = httpsignature.ED25519
	s.KeyID = "primary"
	s.Headers = []string{"digest"}

	err = s.Sign(w.PrivKey, crypto.Hash(0), req)
	return req, err
}

// Register a wallet with Uphold with label
func (w *Wallet) Register(label string) error {
	req, err := w.signRegistration(label)
	if err != nil {
		return err
	}

	body, _, err := submit(w.logger, req)
	if err != nil {
		return err
	}

	var details CardDetails
	err = json.Unmarshal(body, &details)
	if err != nil {
		return err
	}
	w.Info.ProviderID = details.ID.String()
	return nil
}

// SubmitRegistration from a b64 encoded signed string
func (w *Wallet) SubmitRegistration(registrationB64 string) error {
	b, err := base64.StdEncoding.DecodeString(registrationB64)
	if err != nil {
		return err
	}

	var signedTx HTTPSignedRequest
	err = json.Unmarshal(b, &signedTx)
	if err != nil {
		return err
	}

	req, err := newRequest("POST", "/v0/me/cards", nil)
	if err != nil {
		return err
	}

	_, err = signedTx.extract(req)
	if err != nil {
		return err
	}

	body, _, err := submit(w.logger, req)
	if err != nil {
		return err
	}

	var details CardDetails
	err = json.Unmarshal(body, &details)
	if err != nil {
		return err
	}
	w.Info.ProviderID = details.ID.String()
	return nil
}

// PrepareRegistration returns a b64 encoded serialized signed registration suitable for SubmitRegistration
func (w *Wallet) PrepareRegistration(label string) (string, error) {
	req, err := w.signRegistration(label)
	if err != nil {
		return "", err
	}

	httpSignedReq, err := encapsulate(req)
	if err != nil {
		return "", err
	}

	b, err := json.Marshal(&httpSignedReq)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

// CardSettings contains settings corresponding to the Uphold card
type CardSettings struct {
	Protected bool `json:"protected,omitempty"`
}

// CardDetails contains details corresponding to the Uphold card
type CardDetails struct {
	AvailableBalance decimal.Decimal         `json:"available"`
	Balance          decimal.Decimal         `json:"balance"`
	Currency         altcurrency.AltCurrency `json:"currency"`
	ID               uuid.UUID               `json:"id"`
	Settings         CardSettings            `json:"settings"`
}

// GetCardDetails returns the details associated with the wallet's backing Uphold card
func (w *Wallet) GetCardDetails() (*CardDetails, error) {
	req, err := newRequest("GET", "/v0/me/cards/"+w.ProviderID, nil)
	if err != nil {
		return nil, err
	}
	body, _, err := submit(w.logger, req)
	if err != nil {
		return nil, err
	}

	var details CardDetails
	err = json.Unmarshal(body, &details)
	if err != nil {
		return nil, err
	}
	return &details, err
}

// TODO implement func (w *Wallet) UpdatePublicKey() error

// GetWalletInfo returns the info associated with the wallet
func (w *Wallet) GetWalletInfo() walletutils.Info {
	return w.Info
}

type denomination struct {
	Amount   string                   `json:"amount"`
	Currency *altcurrency.AltCurrency `json:"currency"`
}

type transactionRequest struct {
	Denomination denomination `json:"denomination"`
	Destination  string       `json:"destination"`
	Message      string       `json:"message,omitempty"`
}

func (w *Wallet) signTransfer(altc altcurrency.AltCurrency, amount string, destination string, message string) (*http.Request, error) {
	transferReq := transactionRequest{Denomination: denomination{Amount: amount, Currency: &altc}, Destination: destination, Message: message}
	unsignedTransaction, err := json.Marshal(&transferReq)
	if err != nil {
		return nil, err
	}

	req, err := newRequest("POST", "/v0/me/cards/"+w.ProviderID+"/transactions?commit=true", bytes.NewBuffer(unsignedTransaction))
	if err != nil {
		return nil, err
	}

	var s httpsignature.Signature
	s.Algorithm = httpsignature.ED25519
	s.KeyID = "primary"
	s.Headers = []string{"digest"}

	err = s.Sign(w.PrivKey, crypto.Hash(0), req)
	return req, err
}

// PrepareTransaction returns a b64 encoded serialized signed transaction suitable for SubmitTransaction
func (w *Wallet) PrepareTransaction(altcurrency altcurrency.AltCurrency, amount string, destination string, message string) (string, error) {
	req, err := w.signTransfer(altcurrency, amount, destination, message)
	if err != nil {
		return "", err
	}

	httpSignedReq, err := encapsulate(req)
	if err != nil {
		return "", err
	}

	b, err := json.Marshal(&httpSignedReq)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

// Transfer moves funds out of the associated wallet and to the specific destination
func (w *Wallet) Transfer(altcurrency altcurrency.AltCurrency, amount string, destination string) (*walletutils.TransactionInfo, error) {
	req, err := w.signTransfer(altcurrency, amount, destination, "")
	if err != nil {
		return nil, err
	}

	respBody, _, err := submit(w.logger, req)
	if err != nil {
		return nil, err
	}

	var uhResp upholdTransactionResponse
	err = json.Unmarshal(respBody, &uhResp)
	if err != nil {
		return nil, err
	}

	return uhResp.ToTransactionInfo(), nil
}

func (w *Wallet) decodeTransaction(transactionB64 string) (*transactionRequest, error) {
	b, err := base64.StdEncoding.DecodeString(transactionB64)
	if err != nil {
		return nil, err
	}

	var signedTx HTTPSignedRequest
	err = json.Unmarshal(b, &signedTx)
	if err != nil {
		return nil, err
	}

	_, err = govalidator.ValidateStruct(signedTx)
	if err != nil {
		return nil, err
	}

	digestHeader, exists := signedTx.Headers["digest"]
	if !exists {
		return nil, errors.New("A transaction signature must cover the request body via digest")
	}

	var digestInst digest.Instance
	err = digestInst.UnmarshalText([]byte(digestHeader))
	if err != nil {
		return nil, err
	}

	if !digestInst.Verify([]byte(signedTx.Body)) {
		return nil, errors.New("The digest header does not match the included body")
	}

	var req http.Request
	sig, err := signedTx.extract(&req)
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

	valid, err := sig.Verify(w.PubKey, crypto.Hash(0), &req)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, errors.New("The signature is invalid")
	}

	var transaction transactionRequest
	err = json.Unmarshal([]byte(signedTx.Body), &transaction)
	if err != nil {
		return nil, err
	}

	if !govalidator.IsEmail(transaction.Destination) {
		if !validators.IsUUID(transaction.Destination) {
			if !validators.IsBTCAddress(transaction.Destination) {
				if !validators.IsETHAddressNoChecksum(transaction.Destination) {
					return nil, fmt.Errorf("%s is not a valid destination", transaction.Destination)
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

// VerifyTransaction verifies that the transaction is valid
// NOTE VerifyTransaction guards against transactions that seek to exploit parser differences
// such as including additional fields that are not understood by this implementation but may
// be understood by the upstream wallet provider. See DecodeTransaction for details.
func (w *Wallet) VerifyTransaction(transactionB64 string) (*walletutils.TransactionInfo, error) {
	transaction, err := w.decodeTransaction(transactionB64)
	if err != nil {
		return nil, err
	}
	var info walletutils.TransactionInfo
	amount, err := decimal.NewFromString(transaction.Denomination.Amount)
	if err != nil {
		return &info, err
	}
	info.Probi = transaction.Denomination.Currency.ToProbi(amount)
	{
		tmp := *transaction.Denomination.Currency
		info.AltCurrency = &tmp
	}
	info.Destination = transaction.Destination

	return &info, err
}

// VerifyAnonCardTransaction calls VerifyTransaction and checks the currency, amount and destination
func (w *Wallet) VerifyAnonCardTransaction(transactionB64 string, requiredDestination string) (*walletutils.TransactionInfo, error) {
	txInfo, err := w.VerifyTransaction(transactionB64)
	if err != nil {
		return nil, err
	}
	if *txInfo.AltCurrency != altcurrency.BAT {
		return nil, errors.New("only BAT denominated transactions are supported for anon cards")
	}
	if txInfo.Probi.LessThan(decimal.Zero) {
		return nil, errors.New("anon card transaction cannot be for negative BAT")
	}
	if requiredDestination != "" && txInfo.Destination != requiredDestination {
		return nil, errors.New("anon card transactions must have settlement as their destination")
	}

	return txInfo, nil
}

type upholdTransactionResponseDestinationNodeUser struct {
	ID string `json:"id"`
}

type upholdTransactionResponseDestinationNode struct {
	Type string                                       `json:"type"`
	ID   string                                       `json:"id"`
	User upholdTransactionResponseDestinationNodeUser `json:"user"`
}

type upholdTransactionResponseDestination struct {
	Type        string                                   `json:"type"`
	CardID      string                                   `json:"CardId,omitempty"`
	Node        upholdTransactionResponseDestinationNode `json:"node,omitempty"`
	Currency    string                                   `json:"currency"`
	Amount      decimal.Decimal                          `json:"amount"`
	ExchangeFee decimal.Decimal                          `json:"commission"`
	TransferFee decimal.Decimal                          `json:"fee"`
	IsMember    bool                                     `json:"isMember"`
}

type upholdTransactionResponseParams struct {
	TTL int64 `json:"ttl"`
}

type upholdTransactionResponse struct {
	Status       string                               `json:"status"`
	ID           string                               `json:"id"`
	Denomination denomination                         `json:"denomination"`
	Destination  upholdTransactionResponseDestination `json:"destination"`
	Origin       upholdTransactionResponseDestination `json:"origin"`
	Params       upholdTransactionResponseParams      `json:"params"`
	CreatedAt    string                               `json:"createdAt"`
	Message      string                               `json:"message"`
}

func (resp upholdTransactionResponse) ToTransactionInfo() *walletutils.TransactionInfo {
	var txInfo walletutils.TransactionInfo
	txInfo.Probi = resp.Denomination.Currency.ToProbi(decimal.RequireFromString(resp.Denomination.Amount))
	{
		tmp := *resp.Denomination.Currency
		txInfo.AltCurrency = &tmp
	}
	destination := resp.Destination
	destinationNode := destination.Node
	txInfo.UserID = destinationNode.User.ID
	if len(destination.CardID) > 0 {
		txInfo.Destination = destination.CardID
	} else if len(destinationNode.ID) > 0 {
		txInfo.Destination = destinationNode.ID
	}

	if len(resp.Origin.CardID) > 0 {
		txInfo.Source = resp.Origin.CardID
	} else if len(resp.Origin.Node.ID) > 0 {
		txInfo.Source = resp.Origin.Node.ID
	}

	var err error
	txInfo.Time, err = time.Parse(dateFormat, resp.CreatedAt)
	if err != nil {
		log.Fatalf("%s is not a valid ISO 8601 datetime\n", resp.CreatedAt)
	}

	txInfo.DestCurrency = destination.Currency
	txInfo.DestAmount = destination.Amount
	txInfo.TransferFee = destination.TransferFee
	txInfo.ExchangeFee = destination.ExchangeFee
	txInfo.Status = resp.Status
	if txInfo.Status == "pending" {
		txInfo.ValidUntil = time.Now().UTC().Add(time.Duration(resp.Params.TTL) * time.Millisecond)
	}
	txInfo.ID = resp.ID
	txInfo.Note = resp.Message
	txInfo.KYC = destination.IsMember

	return &txInfo
}

// SubmitTransaction submits the base64 encoded transaction for verification but does not move funds
//   unless confirm is set to true.
func (w *Wallet) SubmitTransaction(transactionB64 string, confirm bool) (*walletutils.TransactionInfo, error) {
	_, err := w.VerifyTransaction(transactionB64)
	if err != nil {
		return nil, err
	}

	b, err := base64.StdEncoding.DecodeString(transactionB64)
	if err != nil {
		return nil, err
	}
	var signedTx HTTPSignedRequest
	err = json.Unmarshal(b, &signedTx)
	if err != nil {
		return nil, err
	}

	url := "/v0/me/cards/" + w.ProviderID + "/transactions"
	if confirm {
		url = url + "?commit=true"
	}

	req, err := newRequest("POST", url, nil)
	if err != nil {
		return nil, err
	}

	_, err = signedTx.extract(req)
	if err != nil {
		return nil, err
	}

	respBody, _, err := submit(w.logger, req)
	if err != nil {
		return nil, err
	}

	var uhResp upholdTransactionResponse
	err = json.Unmarshal(respBody, &uhResp)
	if err != nil {
		return nil, err
	}

	return uhResp.ToTransactionInfo(), nil
}

// ConfirmTransaction confirms a previously submitted transaction, moving funds
func (w *Wallet) ConfirmTransaction(id string) (*walletutils.TransactionInfo, error) {
	req, err := newRequest("POST", "/v0/me/cards/"+w.ProviderID+"/transactions/"+id+"/commit", nil)
	if err != nil {
		return nil, err
	}
	body, _, err := submit(w.logger, req)
	if err != nil {
		return nil, err
	}

	var uhResp upholdTransactionResponse
	err = json.Unmarshal(body, &uhResp)
	if err != nil {
		return nil, err
	}

	if uhResp.Destination.Type != "card" && uhResp.Destination.Type != "anonymous" {
		panic("Confirming a non-card transaction is not supported!!!")
	}

	return uhResp.ToTransactionInfo(), nil
}

// GetTransaction returns info about a previously confirmed transaction
func (w *Wallet) GetTransaction(id string) (*walletutils.TransactionInfo, error) {
	req, err := newRequest("GET", "/v0/me/transactions/"+id, nil)
	if err != nil {
		return nil, err
	}
	body, _, err := submit(w.logger, req)
	if err != nil {
		return nil, err
	}

	var uhResp upholdTransactionResponse
	err = json.Unmarshal(body, &uhResp)
	if err != nil {
		return nil, err
	}

	return uhResp.ToTransactionInfo(), nil
}

// ListTransactions for this wallet, pagination not yet supported
func (w *Wallet) ListTransactions(limit int, startDate time.Time) ([]walletutils.TransactionInfo, error) {
	var out []walletutils.TransactionInfo
	if limit > 0 {
		out = make([]walletutils.TransactionInfo, 0, limit)
	}
	var totalTransactions int
	toExit := false
	for {
		req, err := newRequest("GET", "/v0/me/cards/"+w.ProviderID+"/transactions", nil)
		if err != nil {
			return nil, err
		}

		start := len(out)
		stop := start + batchSize
		if limit > 0 && stop >= limit {
			stop = limit - 1
		}
		if totalTransactions != 0 && stop >= totalTransactions {
			stop = totalTransactions - 1
		}

		req.Header.Set("Range", fmt.Sprintf("items=%d-%d", start, stop))
		var body []byte
		var resp *http.Response
		for i := 0; i < listTransactionsRetries; i++ {
			body, resp, err = submit(w.logger, req)
			if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
				if w.logger != nil {
					w.logger.Debug().
						Str("path", "github.com/brave-intl/bat-go/wallet/provider/uphold").
						Str("type", "net.Error").
						Msg("Temporary error occurred, retrying")
				}
				continue
			}
			break
		}
		if err != nil {
			return nil, err
		}

		contentRange := resp.Header.Get("Content-Range")
		parts := strings.Split(contentRange, "/")
		if len(parts) != 2 {
			return nil, errors.New("Invalid Content-Range header returned")
		}
		tmp, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return nil, err
		}
		totalTransactions = int(tmp)

		var uhResp []upholdTransactionResponse
		err = json.Unmarshal(body, &uhResp)
		if err != nil {
			return nil, err
		}

		for i := 0; i < len(uhResp); i++ {
			txInfo := *uhResp[i].ToTransactionInfo()
			if txInfo.Time.Before(startDate) {
				toExit = true
				break
			}
			out = append(out, txInfo)
			if len(out) == limit {
				break
			}
		}

		if len(out) == limit || len(out) == totalTransactions || toExit {
			break
		}
	}
	return out, nil
}

// GetBalance returns the last known balance, if refresh is true then the current balance is fetched
func (w *Wallet) GetBalance(refresh bool) (*wallet.Balance, error) {
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

type createCardAddressRequest struct {
	Network string `json:"network"`
}

type createCardAddressResponse struct {
	ID string `json:"id"`
}

// CreateCardAddress on network, returning the address
func (w *Wallet) CreateCardAddress(network string) (string, error) {
	reqPayload := createCardAddressRequest{Network: network}
	payload, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	req, err := newRequest("POST", fmt.Sprintf("/v0/me/cards/%s/addresses", w.ProviderID), bytes.NewBuffer(payload))
	if err != nil {
		return "", err
	}

	body, _, err := submit(w.logger, req)
	if err != nil {
		return "", err
	}

	var details createCardAddressResponse
	err = json.Unmarshal(body, &details)
	if err != nil {
		return "", err
	}
	return details.ID, nil
}

// FundWallet should fund a given wallet from the donor card (only used in wallet testing)
func FundWallet(destWallet *Wallet, amount decimal.Decimal) (decimal.Decimal, error) {
	var donorInfo walletutils.Info
	donorInfo.Provider = "uphold"
	donorInfo.ProviderID = os.Getenv("DONOR_WALLET_CARD_ID")
	{
		tmp := altcurrency.BAT
		donorInfo.AltCurrency = &tmp
	}
	zero := decimal.NewFromFloat(0)
	donorWalletPublicKeyHex := os.Getenv("DONOR_WALLET_PUBLIC_KEY")
	donorWalletPrivateKeyHex := os.Getenv("DONOR_WALLET_PRIVATE_KEY")
	var donorPublicKey httpsignature.Ed25519PubKey
	var donorPrivateKey ed25519.PrivateKey
	donorPublicKey, err := hex.DecodeString(donorWalletPublicKeyHex)
	if err != nil {
		return zero, err
	}
	donorPrivateKey, err = hex.DecodeString(donorWalletPrivateKeyHex)
	if err != nil {
		return zero, err
	}
	donorWallet := &Wallet{Info: donorInfo, PrivKey: donorPrivateKey, PubKey: donorPublicKey}

	if len(donorWallet.ID) > 0 {
		return zero, errors.New("donor wallet does not have an ID")
	}

	_, err = donorWallet.Transfer(altcurrency.BAT, amount.String(), destWallet.Info.ProviderID)
	if err != nil {
		return zero, err
	}

	balance, err := destWallet.GetBalance(true)
	if err != nil {
		return zero, err
	}

	return balance.TotalProbi, nil
}

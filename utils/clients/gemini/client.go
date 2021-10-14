package gemini

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/clients"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/cryptography"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/google/go-querystring/query"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shengdoushi/base58"
	"github.com/shopspring/decimal"
)

var balanceGauge = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "gemini_account_balance",
	Help: "A gauge of the current account balance in gemini",
})

func init() {
	prometheus.MustRegister(balanceGauge)
}

// WatchGeminiBalance - when called reports the balance to prometheus
func WatchGeminiBalance(ctx context.Context) error {
	logger := logging.Logger(ctx, "WatchGeminiBalance")
	// create a new gemini client
	client, _ := New()

	// get api secret from context
	apiSecret, err := appctx.GetStringFromContext(ctx, appctx.GeminiAPISecretCTXKey)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get gemini api secret")
		return fmt.Errorf("failed to get gemini api secret: %w", err)
	}
	apiKey, err := appctx.GetStringFromContext(ctx, appctx.GeminiAPIKeyCTXKey)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get gemini api key")
		return fmt.Errorf("failed to get gemini api key: %w", err)
	}
	//create a new hmac hasher
	signer := cryptography.NewHMACHasher([]byte(apiSecret))
	for {
		select {
		case <-ctx.Done():
			return nil
		// check every 10 min
		case <-time.After(10 * 60 * time.Second):
			// create the gemini payload
			payload, err := json.Marshal(NewBalancesPayload(nil))
			if err != nil {
				logger.Error().Err(err).Msg("failed to create gemini balance payload")
				// okay to error, retry in 10 min
				continue
			}

			result, err := client.FetchBalances(ctx, apiKey, signer, string(payload))
			if err != nil {
				logger.Error().Err(err).Msg("failed to fetch gemini balance")
				// okay to error, retry in 10 min
				continue
			}
			// dont care about float downsampling from decimal errs
			if result == nil || len(*result) < 1 {
				logger.Error().Msg("gemini result is empty")
				continue
			}
			b := *result
			available, _ := b[0].Available.Float64()
			balanceGauge.Set(available)
		}
	}
}

// PrivateRequestSequence handles the ability to sign a request multiple times
type PrivateRequestSequence struct {
	// the baseline object, corresponds to the signature in the first item
	// must update the nonce before sending otherwise invalid signature will be encountered
	Base       BulkPayoutPayload `json:"base"`
	Signatures []string          `json:"signatures"` // a list of hex encoded singatures
	APIKey     string            `json:"apikey"`     // the api key that corresponds to the checksum server side
	Account    *string           `json:"account,omitempty"`
}

// PayoutPayload contains details about transactions to be confirmed
type PayoutPayload struct {
	TxRef       string          `json:"tx_ref"`
	Amount      decimal.Decimal `json:"amount"`
	Currency    string          `json:"currency"`
	Destination string          `json:"destination"`
	Account     *string         `json:"account,omitempty"`
}

// CheckTxPayload get the tx status payload structure
type CheckTxPayload struct {
	Request string `json:"request"`
	Nonce   int64  `json:"nonce"`
}

// AccountListPayload retrieves all accounts associated with a gemini key
type AccountListPayload struct {
	Request string `json:"request"`
	Nonce   int64  `json:"nonce"`
}

// BalancesPayload retrieves all accounts associated with a gemini key
type BalancesPayload struct {
	Request string  `json:"request"`
	Nonce   int64   `json:"nonce"`
	Account *string `json:"account,omitempty"`
}

// BulkPayoutPayload the payload to be base64'd
type BulkPayoutPayload struct {
	Request       string          `json:"request"`
	Nonce         int64           `json:"nonce"`
	Payouts       []PayoutPayload `json:"payouts"`
	OauthClientID string          `json:"client_id"`
	Account       *string         `json:"account,omitempty"`
}

func nonce() int64 {
	return time.Now().UTC().UnixNano()
}

// SettlementTransactionToPayoutPayload converts to a payout request
func SettlementTransactionToPayoutPayload(tx *settlement.Transaction) PayoutPayload {
	currency := "BAT"
	if tx.Currency != "" {
		currency = tx.Currency
	}
	return PayoutPayload{
		TxRef:       GenerateTxRef(tx),
		Amount:      tx.Amount,
		Currency:    currency,
		Destination: tx.Destination,
	}
}

// GenerateTxRef generates a deterministic transaction reference id for idempotency
func GenerateTxRef(tx *settlement.Transaction) string {
	key := strings.Join([]string{
		tx.SettlementID,
		// if you have to resubmit referrals to get status
		tx.Type,
		tx.Destination,
		tx.Channel,
	}, "_")
	bytes := sha256.Sum256([]byte(key))
	refID := base58.Encode(bytes[:], base58.IPFSAlphabet)
	return refID
}

// NewBulkPayoutPayload generate a new bulk payout payload
func NewBulkPayoutPayload(account *string, oauthClientID string, payouts *[]PayoutPayload) BulkPayoutPayload {
	if oauthClientID == "" {
		panic("unable to sign a payload without an oauth client id (GEMINI_CLIENT_ID)")
	}
	return BulkPayoutPayload{
		Account:       account,
		OauthClientID: oauthClientID,
		Request:       "/v1/payments/bulkPay",
		Nonce:         nonce(),
		Payouts:       *payouts,
	}
}

// NewCheckTxPayload generate a new payload for the check tx api
func NewCheckTxPayload(url string) CheckTxPayload {
	return CheckTxPayload{
		Request: url,
		Nonce:   nonce(),
	}
}

// NewAccountListPayload generate a new account list payload
func NewAccountListPayload() AccountListPayload {
	return AccountListPayload{
		Request: "/v1/account/list",
		Nonce:   nonce(),
	}
}

// NewBalancesPayload generate a new account list payload
func NewBalancesPayload(account *string) BalancesPayload {
	return BalancesPayload{
		Request: "/v1/balances",
		Nonce:   nonce(),
		Account: account,
	}
}

// PayoutResult contains details about a newly created or fetched issuer
type PayoutResult struct {
	Result      string           `json:"result"` // OK or Error
	TxRef       string           `json:"tx_ref"`
	Amount      *decimal.Decimal `json:"amount"`
	Currency    *string          `json:"currency"`
	Destination *string          `json:"destination"`
	Status      *string          `json:"status"`
	Reason      *string          `json:"reason"`
}

// Balance holds balance info
type Balance struct {
	Type                   string          `json:"type"`
	Currency               string          `json:"currency"`
	Amount                 decimal.Decimal `json:"amount"`
	Available              decimal.Decimal `json:"available"`
	AvailableForWithdrawal decimal.Decimal `json:"availableForWithdrawal"`
}

// Account holds account info
type Account struct {
	Name           string `json:"name"`
	Class          string `json:"account"`
	Type           string `json:"type"`
	CounterpartyID string `json:"counterparty_id"`
	CreatedAt      int64  `json:"created"`
}

// GenerateLog creates a log
func (pr PayoutResult) GenerateLog() string {
	if pr.Result == "OK" {
		return ""
	}
	return strings.Join([]string{pr.Result, pr.TxRef, *pr.Status, *pr.Reason}, ": ")
}

// Client abstracts over the underlying client
type Client interface {
	// ValidateAccount - given a verificationToken validate the token is authentic and get the unique account id
	ValidateAccount(ctx context.Context, verificationToken, recipientID string) (string, error)
	// FetchAccountList requests account information to scope future requests
	FetchAccountList(ctx context.Context, APIKey string, signer cryptography.HMACKey, payload string) (*[]Account, error)
	// FetchBalances requests balance information for a given account
	FetchBalances(ctx context.Context, APIKey string, signer cryptography.HMACKey, payload string) (*[]Balance, error)
	// UploadBulkPayout posts a signed bulk layout to gemini
	UploadBulkPayout(ctx context.Context, APIKey string, signer cryptography.HMACKey, payload string) (*[]PayoutResult, error)
	// CheckTxStatus checks the status of a transaction
	CheckTxStatus(ctx context.Context, APIKEY string, clientID string, txRef string) (*PayoutResult, error)
}

// HTTPClient wraps http.Client for interacting with the cbr server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
}

// Conf some common gemini configuration values
type Conf struct {
	ClientID string
	APIKey   string
	Secret   string
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (Client, error) {
	serverEnvKey := "GEMINI_SERVER"
	serverURL := os.Getenv(serverEnvKey)
	if len(serverURL) == 0 {
		return nil, errors.New(serverEnvKey + " was empty")
	}
	proxy := os.Getenv("HTTP_PROXY")
	client, err := clients.NewWithProxy("gemini", serverURL, os.Getenv("GEMINI_TOKEN"), proxy)
	if err != nil {
		return nil, err
	}
	return NewClientWithPrometheus(&HTTPClient{client}, "gemini_client"), err
}

func setHeaders(
	req *http.Request,
	APIKey string,
	signer *cryptography.HMACKey,
	payload string,
	submitType string,
) error {
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Content-Length", "0")
	req.Header.Set("Cache-Control", "no-cache")
	if payload != "" {
		// base64 encode the payload
		req.Header.Set("X-GEMINI-PAYLOAD", base64.StdEncoding.EncodeToString([]byte(payload)))
	}
	if submitType != "oauth" {
		// do not send when oauth
		req.Header.Set("X-GEMINI-APIKEY", APIKey)
	}
	return setPrivateRequestHeaders(
		req,
		APIKey,
		signer,
		payload,
		submitType,
	)
}

func setPrivateRequestHeaders(
	req *http.Request,
	APIKey string,
	signer *cryptography.HMACKey,
	payload string,
	submitType string,
) error {
	if submitType == "hmac" {
		if signer == nil {
			return errors.New("GEMINI_SUBMIT_TYPE set to 'hmac' but no signer provided")
		}
		signs := *signer
		// only set if sending an hmac salt
		signature, err := signs.HMACSha384([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
		if err != nil {
			return err
		}
		req.Header.Set("X-GEMINI-SIGNATURE", hex.EncodeToString(signature))
	}
	return nil
}

// CheckTxStatus uploads the bulk payout for gemini
func (c *HTTPClient) CheckTxStatus(
	ctx context.Context,
	APIKey string,
	clientID string,
	txRef string,
) (*PayoutResult, error) {
	urlPath := fmt.Sprintf("/v1/payment/%s/%s", clientID, txRef)
	req, err := c.client.NewRequest(ctx, "GET", urlPath, nil, nil)
	if err != nil {
		return nil, err
	}

	// create the gemini payload
	payload, err := json.Marshal(NewCheckTxPayload(urlPath))
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini payload for api: %w", err)
	}

	// get api secret from context
	apiSecret, err := appctx.GetStringFromContext(ctx, appctx.GeminiAPISecretCTXKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get gemini signing secret from ctx: %w", err)
	}
	//create a new hmac hasher
	signer := cryptography.NewHMACHasher([]byte(apiSecret))

	err = setHeaders(req, APIKey, &signer, string(payload), "hmac")
	if err != nil {
		return nil, err
	}

	var body PayoutResult
	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, err
	}
	return &body, err
}

// UploadBulkPayout uploads the bulk payout for gemini
func (c *HTTPClient) UploadBulkPayout(
	ctx context.Context,
	APIKey string,
	signer cryptography.HMACKey,
	payload string,
) (*[]PayoutResult, error) {
	req, err := c.client.NewRequest(ctx, "POST", "/v1/payments/bulkPay", nil, nil)
	if err != nil {
		return nil, err
	}
	err = setHeaders(req, APIKey, &signer, payload, os.Getenv("GEMINI_SUBMIT_TYPE"))
	if err != nil {
		return nil, err
	}

	var body []PayoutResult
	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, err
	}
	return &body, err
}

// ValidateAccountReq - request structure for inputs to validate account client call
type ValidateAccountReq struct {
	Token       string `url:"token"`
	RecipientID string `url:"recipient_id"`
}

// GenerateQueryString - implement the QueryStringBody interface
func (v *ValidateAccountReq) GenerateQueryString() (url.Values, error) {
	return query.Values(v)
}

// ValidateAccountRes - request structure for inputs to validate account client call
type ValidateAccountRes struct {
	ID string `json:"id"`
}

// ValidateAccount - given a verificationToken validate the token is authentic and get the unique account id
func (c *HTTPClient) ValidateAccount(ctx context.Context, verificationToken, recipientID string) (string, error) {
	// create the query string parameters
	var (
		res = new(ValidateAccountRes)
	)

	// create the request
	req, err := c.client.NewRequest(ctx, "POST", "/v1/account/validate", nil, &ValidateAccountReq{
		Token:       verificationToken,
		RecipientID: recipientID,
	})

	if err != nil {
		return "", err
	}

	_, err = c.client.Do(ctx, req, res)
	if err != nil {
		return "", err
	}
	return res.ID, nil
}

// FetchAccountList fetches the list of accounts associated with the given api key
func (c *HTTPClient) FetchAccountList(
	ctx context.Context,
	APIKey string,
	signer cryptography.HMACKey,
	payload string,
) (*[]Account, error) {
	req, err := c.client.NewRequest(ctx, "POST", "/v1/account/list", nil, nil)
	if err != nil {
		return nil, err
	}
	err = setHeaders(req, APIKey, &signer, payload, os.Getenv("GEMINI_SUBMIT_TYPE"))
	if err != nil {
		return nil, err
	}

	var body []Account
	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, err
	}
	return &body, err
}

// FetchBalances fetches the list of accounts associated with the given api key
func (c *HTTPClient) FetchBalances(
	ctx context.Context,
	APIKey string,
	signer cryptography.HMACKey,
	payload string,
) (*[]Balance, error) {
	req, err := c.client.NewRequest(ctx, "POST", "/v1/balances", nil, nil)
	if err != nil {
		return nil, err
	}
	err = setHeaders(req, APIKey, &signer, payload, os.Getenv("GEMINI_SUBMIT_TYPE"))
	if err != nil {
		return nil, err
	}

	var body []Balance
	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, err
	}
	return &body, err
}

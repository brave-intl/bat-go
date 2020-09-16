package gemini

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/clients"
	"github.com/brave-intl/bat-go/utils/cryptography"
	"github.com/shengdoushi/base58"
	"github.com/shopspring/decimal"
)

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
	return BulkPayoutPayload{
		Account:       account,
		OauthClientID: oauthClientID,
		Request:       "/v1/payments/bulkPay",
		Nonce:         nonce(),
		Payouts:       *payouts,
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
	// FetchAccountList requests account information to scope future requests
	FetchAccountList(ctx context.Context, APIKey string, signer cryptography.HMACKey, payload string) (*[]Account, error)
	// FetchBalances requests balance information for a given account
	FetchBalances(ctx context.Context, APIKey string, signer cryptography.HMACKey, payload string) (*[]Balance, error)
	// UploadBulkPayout posts a signed bulk layout to gemini
	UploadBulkPayout(ctx context.Context, APIKey string, signer cryptography.HMACKey, payload string) (*[]PayoutResult, error)
}

// HTTPClient wraps http.Client for interacting with the cbr server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
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

func setPrivateRequestHeaders(
	req *http.Request,
	APIKey string,
	signer cryptography.HMACKey,
	payload string,
) error {
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Content-Length", "0")
	req.Header.Set("X-GEMINI-PAYLOAD", payload)
	if os.Getenv("GEMINI_SUBMIT_TYPE") != "oauth" {
		signature, err := signer.HMACSha384([]byte(payload))
		if err != nil {
			return err
		}
		req.Header.Set("X-GEMINI-APIKEY", APIKey)
		req.Header.Set("X-GEMINI-SIGNATURE", hex.EncodeToString(signature))
	}
	req.Header.Set("Cache-Control", "no-cache")
	return nil
}

// UploadBulkPayout uploads the bulk payout for gemini
func (c *HTTPClient) UploadBulkPayout(
	ctx context.Context,
	APIKey string,
	signer cryptography.HMACKey,
	payload string,
) (*[]PayoutResult, error) {
	req, err := c.client.NewRequest(ctx, "POST", "/v1/payments/bulkPay", nil)
	if err != nil {
		return nil, err
	}
	err = setPrivateRequestHeaders(req, APIKey, signer, payload)
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

// FetchAccountList fetches the list of accounts associated with the given api key
func (c *HTTPClient) FetchAccountList(
	ctx context.Context,
	APIKey string,
	signer cryptography.HMACKey,
	payload string,
) (*[]Account, error) {
	req, err := c.client.NewRequest(ctx, "POST", "/v1/account/list", nil)
	if err != nil {
		return nil, err
	}
	err = setPrivateRequestHeaders(req, APIKey, signer, payload)
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
	req, err := c.client.NewRequest(ctx, "POST", "/v1/balances", nil)
	if err != nil {
		return nil, err
	}
	err = setPrivateRequestHeaders(req, APIKey, signer, payload)
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

package bitflyer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/shengdoushi/base58"
	"github.com/shopspring/decimal"
)

// Quote returns a quote of BAT prices
type Quote struct {
	ProductCode  string          `json:"product_code"`
	MainCurrency string          `json:"main_currency"`
	SubCurrency  string          `json:"sub_currency"`
	Rate         decimal.Decimal `json:"rate"`
	PriceToken   string          `json:"price_token"`
}

// QuoteQuery holds the query params for the quote
type QuoteQuery struct {
	ProductCode string `url:"product_code,omitempty"`
}

// WithdrawToDepositIDPayload holds a single withdrawal request
type WithdrawToDepositIDPayload struct {
	CurrencyCode string  `json:"currency_code"`
	Amount       float64 `json:"amount"`
	DryRun       *bool   `json:"dry_run,omitempty"`
	DepositID    string  `json:"deposit_id"`
	TransferID   string  `json:"transfer_id"`
	SourceFrom   string  `json:"source_from"`
}

// WithdrawToDepositIDBulkPayload holds all WithdrawToDepositIDPayload(s) for a single bulk request
type WithdrawToDepositIDBulkPayload struct {
	DryRun       bool                         `json:"dry_run"`
	Withdrawals  []WithdrawToDepositIDPayload `json:"withdrawals"`
	PriceToken   string                       `json:"price_token"`
	DryRunOption *DryRunOption                `json:"dry_run_option"`
}

// WithdrawToDepositIDResponse holds a single withdrawal request
type WithdrawToDepositIDResponse struct {
	CurrencyCode string          `json:"currency_code"`
	Amount       decimal.Decimal `json:"amount"`
	Message      string          `json:"message"`
	Status       string          `json:"transfer_status"`
	TransferID   string          `json:"transfer_id"`
}

// TokenPayload holds the data needed to get a new token
type TokenPayload struct {
	GrantType         string `json:"grant_type"`
	ClientID          string `json:"client_id"`
	ClientSecret      string `json:"client_secret"`
	ExtraClientSecret string `json:"extra_client_secret"`
}

// TokenResponse holds the response from refreshing a token
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	AccountHash  string `json:"account_hash"`
	TokenType    string `json:"token_type"`
}

// DryRunOption holds options for dry running a transaction
type DryRunOption struct {
	RequestAPITransferStatus string `json:"request_api_transfer_status"`
	ProcessTimeSec           uint   `json:"process_time_sec"`
	StatusAPITransferStatus  string `json:"status_api_transfer_status"`
}

// NewWithdrawToDepositIDBulkPayload creates a bulk request
func NewWithdrawToDepositIDBulkPayload(dryRunOptions *DryRunOption, priceToken string, withdrawals *[]WithdrawToDepositIDPayload) *WithdrawToDepositIDBulkPayload {
	dryRun := false
	if dryRunOptions != nil {
		dryRun = true
		enum := "ENUM"
		if dryRunOptions.RequestAPITransferStatus != enum {
			dryRunOptions.RequestAPITransferStatus = enum
		}
		if dryRunOptions.StatusAPITransferStatus != enum {
			dryRunOptions.StatusAPITransferStatus = enum
		}
	}
	return &WithdrawToDepositIDBulkPayload{
		PriceToken:   priceToken,
		Withdrawals:  *withdrawals,
		DryRun:       dryRun,
		DryRunOption: dryRunOptions,
	}
}

// WithdrawToDepositIDBulkResponse holds info about the status of the bulk settlements
type WithdrawToDepositIDBulkResponse struct {
	DryRun      bool                          `json:"dry_run"`
	Withdrawals []WithdrawToDepositIDResponse `json:"withdrawals"`
}

// NewWithdrawsFromTxs creates an array of withdrawal requests
func NewWithdrawsFromTxs(
	sourceFrom string,
	txs *[]settlement.Transaction,
) (*[]WithdrawToDepositIDPayload, error) {
	withdrawals := []WithdrawToDepositIDPayload{}
	if sourceFrom == "" {
		sourceFrom = "self"
	}
	for _, tx := range *txs {
		probi := altcurrency.BAT.FromProbi(tx.Probi)
		if probi.Exponent() > 8 {
			return nil, errors.New("cannot convert float exactly")
		}
		f64, _ := probi.Float64()
		withdrawals = append(withdrawals, WithdrawToDepositIDPayload{
			CurrencyCode: "BAT",
			Amount:       f64,
			DepositID:    tx.Destination,
			TransferID:   GenerateTransferID(&tx),
			SourceFrom:   sourceFrom,
		})
	}
	return &withdrawals, nil
}

// GenerateTransferID generates a deterministic transaction reference id for idempotency
func GenerateTransferID(tx *settlement.Transaction) string {
	inputs := []string{
		tx.SettlementID,
		tx.Destination,
		// tx.Channel, // all channels are grouped together
	}
	key := strings.Join(inputs, "_")
	bytes := sha256.Sum256([]byte(key))
	refID := base58.Encode(bytes[:], base58.IPFSAlphabet)
	return refID
}

// Client abstracts over the underlying client
type Client interface {
	// FetchQuote gets a quote of BAT to JPY
	FetchQuote(ctx context.Context, productCode string) (*Quote, error)
	// UploadBulkPayout posts a signed bulk layout to bitflyer
	UploadBulkPayout(ctx context.Context, payload WithdrawToDepositIDBulkPayload) (*WithdrawToDepositIDBulkResponse, error)
	// CheckPayoutStatus checks the status of a transaction
	CheckPayoutStatus(ctx context.Context, payload WithdrawToDepositIDBulkPayload) (*WithdrawToDepositIDBulkResponse, error)
	// RefreshToken refreshes the token belonging to the provided secret values
	RefreshToken(ctx context.Context, payload TokenPayload) (*TokenResponse, error)
	// SetAuthToken sets the auth token on underlying client object
	SetAuthToken(authToken string)
}

// HTTPClient wraps http.Client for interacting with the cbr server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (Client, error) {
	serverEnvKey := "BITFLYER_SERVER"
	serverURL := os.Getenv(serverEnvKey)
	if len(serverURL) == 0 {
		return nil, errors.New(serverEnvKey + " was empty")
	}
	proxy := os.Getenv("HTTP_PROXY")
	client, err := clients.NewWithProxy("bitflyer", serverURL, os.Getenv("BITFLYER_TOKEN"), proxy)
	if err != nil {
		return nil, err
	}
	return NewClientWithPrometheus(&HTTPClient{client}, "bitflyer_client"), err
}

// SetAuthToken sets the auth token
func (c *HTTPClient) SetAuthToken(
	authToken string,
) {
	c.client.AuthToken = authToken
}

// FetchQuote fetches prices for determining constraints
func (c *HTTPClient) FetchQuote(
	ctx context.Context,
	productCode string,
) (*Quote, error) {
	req, err := c.client.NewRequest(ctx, "GET", "/api/link/v1/getprice", QuoteQuery{
		ProductCode: productCode,
	})
	if err != nil {
		return nil, err
	}
	var body Quote
	resp, err := c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, handleBitflyerError(err, req, resp)
	}
	return &body, nil
}

// UploadBulkPayout uploads payouts to bitflyer
func (c *HTTPClient) UploadBulkPayout(
	ctx context.Context,
	payload WithdrawToDepositIDBulkPayload,
) (*WithdrawToDepositIDBulkResponse, error) {
	req, err := c.client.NewRequest(ctx, http.MethodPost, "/api/link/v1/coin/withdraw-to-deposit-id/bulk-request", payload)
	if err != nil {
		return nil, err
	}
	c.setupRequestHeaders(req)
	var body WithdrawToDepositIDBulkResponse
	resp, err := c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, handleBitflyerError(err, req, resp)
	}
	return &body, nil
}

// CheckPayoutStatus checks bitflyer transaction status
func (c *HTTPClient) CheckPayoutStatus(
	ctx context.Context,
	payload WithdrawToDepositIDBulkPayload,
) (*WithdrawToDepositIDBulkResponse, error) {
	req, err := c.client.NewRequest(ctx, http.MethodPost, "/api/link/v1/coin/withdraw-to-deposit-id/bulk-status", payload)
	if err != nil {
		return nil, err
	}
	c.setupRequestHeaders(req)
	var body WithdrawToDepositIDBulkResponse
	resp, err := c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, handleBitflyerError(err, req, resp)
	}
	return &body, nil
}

// RefreshToken gets a new token from bitflyer
func (c *HTTPClient) RefreshToken(
	ctx context.Context,
	payload TokenPayload,
) (*TokenResponse, error) {
	req, err := c.client.NewRequest(ctx, http.MethodPost, "/api/link/v1/token", payload)
	if err != nil {
		return nil, err
	}
	c.setupRequestHeaders(req)
	var body TokenResponse
	resp, err := c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, handleBitflyerError(err, req, resp)
	}
	return &body, nil
}

func (c *HTTPClient) setupRequestHeaders(req *http.Request) {
	req.Header.Set("authorization", "Bearer "+c.client.AuthToken)
	req.Header.Set("content-type", "application/json")
}

func handleBitflyerError(e error, req *http.Request, resp *http.Response) error {
	if resp == nil {
		return e
	}
	b, err := requestutils.Read(resp.Body)
	if err != nil {
		return err
	}
	var bfError clients.BitflyerError
	err = json.Unmarshal(b, &bfError)
	if err != nil {
		return err
	}
	if len(bfError.Label) == 0 {
		return e
	}
	return bfError
}

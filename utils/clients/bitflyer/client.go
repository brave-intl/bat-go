package bitflyer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/clients"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/shengdoushi/base58"
	"github.com/shopspring/decimal"
)

var (
	bitflyerNS = uuid.Must(uuid.FromString("6ff61d64-7bcd-4ad7-aed8-25752b7f332e"))
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
	Message      string  `json:"message"`
	TransferID   string  `json:"transfer_id"`
	SourceFrom   string  `json:"source_from"`
}

// WithdrawToDepositIDBulkPayload holds all WithdrawToDepositIDPayload(s) for a single bulk request
type WithdrawToDepositIDBulkPayload struct {
	DryRun      bool                         `json:"dry_run"`
	Withdrawals []WithdrawToDepositIDPayload `json:"withdrawals"`
	PriceToken  string                       `json:"price_token"`
}

// WithdrawToDepositIDResponse holds a single withdrawal request
type WithdrawToDepositIDResponse struct {
	CurrencyCode string          `json:"currency_code"`
	Amount       decimal.Decimal `json:"amount"`
	Message      string          `json:"message"`
	Status       string          `json:"transfer_status"`
	TransferID   string          `json:"transfer_id"`
}

// NewWithdrawToDepositIDBulkPayload creates a bulk request
func NewWithdrawToDepositIDBulkPayload(dryRun bool, priceToken string, withdrawals *[]WithdrawToDepositIDPayload) *WithdrawToDepositIDBulkPayload {
	return &WithdrawToDepositIDBulkPayload{
		DryRun:      dryRun,
		PriceToken:  priceToken,
		Withdrawals: *withdrawals,
	}
}

// WithdrawToDepositIDBulkResponse holds info about the status of the bulk settlements
type WithdrawToDepositIDBulkResponse struct {
	DryRun      bool                          `json:"dry_run"`
	Withdrawals []WithdrawToDepositIDResponse `json:"withdrawals"`
}

// NewWithdrawsFromTxs creates an array of withdrawal requests
func NewWithdrawsFromTxs(sourceFrom string, txs *[]settlement.Transaction) (*[]WithdrawToDepositIDPayload, error) {
	withdrawals := []WithdrawToDepositIDPayload{}
	if sourceFrom == "" {
		sourceFrom = "self"
	}
	for _, tx := range *txs {
		if tx.Amount.Exponent() > 8 {
			return nil, errors.New("cannot convert float exactly")
		}
		f64, _ := tx.Amount.Float64()
		withdrawals = append(withdrawals, WithdrawToDepositIDPayload{
			CurrencyCode: "BAT",
			Amount:       f64,
			DepositID:    tx.Destination,
			Message:      tx.Note,
			TransferID:   GenerateTransferID(&tx),
			SourceFrom:   sourceFrom,
		})
	}
	return &withdrawals, nil
}

// GenerateTransferID generates a deterministic transaction reference id for idempotency
func GenerateTransferID(tx *settlement.Transaction) string {
	key := strings.Join([]string{
		tx.SettlementID,
		tx.Destination,
		// tx.Channel, // all channels are grouped together
	}, "_")
	bytes := sha256.Sum256([]byte(key))
	refID := base58.Encode(bytes[:], base58.IPFSAlphabet)
	return refID
}

// GenerateSettlementID generates a deterministic transaction reference id for idempotency
func GenerateSettlementID(tx *settlement.Transaction) string {
	key := strings.Join([]string{
		tx.SettlementID,
		tx.Destination,
		tx.Publisher,
		tx.Channel,
	}, "_")
	bytes := sha256.Sum256([]byte(key))
	refID := base58.Encode(bytes[:], base58.IPFSAlphabet)
	return refID
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
	// FetchQuote gets a quote of BAT to JPY
	FetchQuote(ctx context.Context, productCode string) (*Quote, error)
	// // FetchAccountList requests account information to scope future requests
	// FetchAccountList(ctx context.Context, APIKey string, signer cryptography.HMACKey, payload string) (*[]Account, error)
	// // FetchBalances requests balance information for a given account
	// FetchBalances(ctx context.Context, APIKey string, signer cryptography.HMACKey, payload /string) (*[]Balance, error)
	// UploadBulkPayout posts a signed bulk layout to bitflyer
	UploadBulkPayout(ctx context.Context, APIKey string, payload WithdrawToDepositIDBulkPayload) (*WithdrawToDepositIDBulkResponse, error)
	// CheckPayoutStatus checks the status of a transaction
	CheckPayoutStatus(ctx context.Context, APIKey string, payload WithdrawToDepositIDBulkPayload) (*WithdrawToDepositIDBulkResponse, error)
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
	APIKey string,
	payload WithdrawToDepositIDBulkPayload,
) (*WithdrawToDepositIDBulkResponse, error) {
	req, err := c.client.NewRequest(ctx, http.MethodPost, "/api/link/v1/coin/withdraw-to-deposit-id/bulk-request", payload)
	if err != nil {
		return nil, err
	}
	setupRequestHeaders(req, APIKey)
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
	APIKey string,
	payload WithdrawToDepositIDBulkPayload,
) (*WithdrawToDepositIDBulkResponse, error) {
	req, err := c.client.NewRequest(ctx, http.MethodPost, "/api/link/v1/coin/withdraw-to-deposit-id/bulk-status", payload)
	if err != nil {
		return nil, err
	}
	setupRequestHeaders(req, APIKey)
	var body WithdrawToDepositIDBulkResponse
	resp, err := c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, handleBitflyerError(err, req, resp)
	}
	return &body, nil
}

func setupRequestHeaders(req *http.Request, APIKey string) {
	req.Header.Set("authorization", "Bearer "+APIKey)
	req.Header.Set("content-type", "application/json")
}

func handleBitflyerError(e error, req *http.Request, resp *http.Response) error {
	b, err := ioutil.ReadAll(resp.Body)
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
	bundle, ok := e.(*errorutils.ErrorBundle)
	if !ok {
		return e
	}
	state, ok := bundle.Data().(clients.HTTPState)
	if !ok {
		return e
	}
	return clients.NewHTTPError(
		err,
		state.Path,
		bundle.Error(),
		state.Status,
		bfError,
	)
}

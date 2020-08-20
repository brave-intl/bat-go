package gemini

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/clients"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/shopspring/decimal"
)

// PrivateRequest holds all of the requisite info to complete a gemini bulk payout
type PrivateRequest struct {
	Signature    string                   `json:"signature"`
	Payload      string                   `json:"payload"` // base64'd
	APIKey       string                   `json:"api_key"`
	Transactions []settlement.Transaction `json:"transaction"`
}

// PayoutRequest contains details about transactions to be confirmed
type PayoutRequest struct {
	TxRef       string          `json:"tx_ref"`
	Amount      decimal.Decimal `json:"amount"`
	Currency    string          `json:"currency"`
	Destination string          `json:"destination"`
}

// BulkPayoutRequest the payload to be base64'd
type BulkPayoutRequest struct {
	Request string          `json:"request"`
	Nonce   int64           `json:"nonce"`
	Payouts []PayoutRequest `json:"payouts"`
}

// PayoutResponse contains details about a newly created or fetched issuer
type PayoutResponse struct {
	Result      string           `json:"result"` // OK or Error
	TxRef       string           `json:"tx_ref"`
	Amount      *decimal.Decimal `json:"amount"`
	Currency    *string          `json:"currency"`
	Destination *string          `json:"destination"`
	Status      *string          `json:"status"`
	Reason      *string          `json:"reason"`
}

// GenerateLog creates a log
func (pr PayoutResponse) GenerateLog() string {
	if pr.Result == "OK" {
		return ""
	}
	return strings.Join([]string{pr.Result, pr.TxRef, *pr.Status, *pr.Reason}, ": ")
}

// Client abstracts over the underlying client
type Client interface {
	UploadBulkPayout(ctx context.Context, request PrivateRequest) (*[]PayoutResponse, error)
}

// HTTPClient wraps http.Client for interacting with the cbr server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (Client, error) {
	serverEnvKey := "GEMINI_SERVER"
	serverURL := os.Getenv("GEMINI_SERVER")
	if len(serverURL) == 0 {
		return nil, errors.New(serverEnvKey + " was empty")
	}
	client, err := clients.New(serverURL, os.Getenv("GEMINI_TOKEN"))
	if err != nil {
		return nil, err
	}
	return NewClientWithPrometheus(&HTTPClient{client}, "gemini_client"), err
}

// UploadBulkPayout uploads the bulk payout for gemini
func (c *HTTPClient) UploadBulkPayout(ctx context.Context, request PrivateRequest) (*[]PayoutResponse, error) {
	req, err := c.client.NewRequest(ctx, "POST", "v1/payments/bulkPay", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Content-Length", "0")
	req.Header.Set("X-GEMINI-APIKEY", request.APIKey)
	req.Header.Set("X-GEMINI-PAYLOAD", request.Payload)
	req.Header.Set("X-GEMINI-SIGNATURE", request.Signature)
	req.Header.Set("Cache-Control", "no-cache")

	res, err := c.client.Do(ctx, req, nil)
	if err != nil {
		return nil, err
	}
	var response []PayoutResponse
	err = requestutils.ReadJSON(res.Body, &response)

	return &response, err
}

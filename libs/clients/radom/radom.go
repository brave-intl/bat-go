package radom

import (
	"context"
	"crypto/subtle"
	"errors"

	"github.com/shopspring/decimal"

	"github.com/brave-intl/bat-go/libs/clients"
)

var (
	ErrInvalidMetadataKey = errors.New("failed to get key from radom metadata")
)

// CheckoutSessionRequest represents a request to create a checkout session.
type CheckoutSessionRequest struct {
	SuccessURL     string                 `json:"successUrl"`
	SellerAddress  string                 `json:"sellerAddress"`
	CancelURL      string                 `json:"cancelUrl"`
	Currency       string                 `json:"currency"`
	ExpiresAt      int64                  `json:"expiresAt"` // in unix seconds
	AcceptedChains []int64                `json:"acceptedChains"`
	AcceptedTokens []AcceptedToken        `json:"acceptedTokens"`
	LineItems      []LineItem             `json:"lineItems"`
	Metadata       Metadata               `json:"metadata"`
	Customizations map[string]interface{} `json:"customizations"`
	Total          decimal.Decimal        `json:"total"`
}

// CheckoutSessionResponse represents the result of submission of a checkout session.
type CheckoutSessionResponse struct {
	SessionID  string `json:"checkoutSessionId"`
	SessionURL string `json:"checkoutSessionUrl"`
}

// AcceptedToken represents the accepted token in the checkout session.
type AcceptedToken struct {
	ChainID      int64  `json:"chainId"`
	TokenAddress string `json:"tokenAddress"`
}

// LineItem is a line item for a checkout session request.
type LineItem struct {
	ProductID string                 `json:"productId"`
	ItemData  map[string]interface{} `json:"itemData"`
}

// Metadata represents metaadata in a checkout session request.
type Metadata []KeyValue

// Get allows returns a value based on the key from the Radom metadata.
func (m Metadata) Get(key string) (map[string]interface{}, error) {
	for _, v := range m {
		if subtle.ConstantTimeCompare([]byte(key), []byte(v.Key)) == 1 {
			return v.Value, nil
		}
	}

	return nil, ErrInvalidMetadataKey
}

// KeyValue represents a key-value metadata pair.
type KeyValue struct {
	Key   string                 `json:"key"`
	Value map[string]interface{} `json:"value"`
}

// WebhookRequest represents a webhook payload.
type WebhookRequest struct {
	EventName            string   `json:"eventName"`
	TransactionHash      string   `json:"transactionHash"`
	SellerAddress        string   `json:"sellerAddress"`
	CustomerAddress      string   `json:"customerAddress"`
	PaymentHash          string   `json:"paymentHash"`
	PaymentToken         string   `json:"paymentToken"`
	PaymentAmount        string   `json:"paymentAmount"`
	BlockNumber          int64    `json:"blockNumber"`
	TransactionTimestamp int64    `json:"transactionTimestamp"`
	ChainID              int64    `json:"chainId"`
	Metadata             Metadata `json:"metadata"`
}

// Client communicates with Radom.
type Client struct {
	client *clients.SimpleHTTPClient
	chains []int64
	tokens []AcceptedToken
}

// New returns a ready to use Client.
func New(srvURL, secret, proxyAddr string, chains []int64, tokens []AcceptedToken) (*Client, error) {
	return newClient(srvURL, secret, proxyAddr, chains, tokens)
}

func NewInstrumented(srvURL, secret, proxyAddr string, chains []int64, tokens []AcceptedToken) (*InstrumentedClient, error) {
	cl, err := newClient(srvURL, secret, proxyAddr, chains, tokens)
	if err != nil {
		return nil, err
	}

	return newInstrucmentedClient("radom_client", cl), nil
}

func newClient(srvURL, secret, proxyAddr string, chains []int64, tokens []AcceptedToken) (*Client, error) {
	client, err := clients.NewWithProxy("radom", srvURL, secret, proxyAddr)
	if err != nil {
		return nil, err
	}

	return &Client{client: client, chains: chains, tokens: tokens}, nil
}

// CreateCheckoutSession creates a Radom checkout session.
func (c *Client) CreateCheckoutSession(
	ctx context.Context,
	req *CheckoutSessionRequest,
) (*CheckoutSessionResponse, error) {
	req.AcceptedChains = c.chains
	req.AcceptedTokens = c.tokens

	return nil, errors.New("not implemented")
}

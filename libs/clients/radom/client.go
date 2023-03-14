package radom

import (
	"crypto/subtle"
	"errors"
	"os"

	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/shopspring/decimal"
)

// KeyValue is the structure of the key value pairs
type KeyValue struct {
	Key   string                 `json:"key"`
	Value map[string]interface{} `json:"value"`
}

// Metadata is the structure of metadata
type Metadata []KeyValue

// Get allows one to get a value based on key from the radom metadata
func (rm *Metadata) Get(key string) (map[string]interface{}, error) {
	for _, v := range []KeyValue(*rm) {
		if subtle.ConstantTimeCompare([]byte(key), []byte(v.Key)) == 1 {
			return v.Value, nil
		}
	}
	return nil, errors.New("failed to get key from radom metadata")
}

// WebhookRequest is the request from radom webhooks
type WebhookRequest struct {
	EventName            string   `json:"eventName"`
	BlockNumber          int64    `json:"blockNumber"`
	TransactionHash      string   `json:"transactionHash"`
	TransactionTimestamp int64    `json:"transactionTimestamp"`
	SellerAddress        string   `json:"sellerAddress"`
	CustomerAddress      string   `json:"customerAddress"`
	PaymentHash          string   `json:"paymentHash"`
	ChainID              int64    `json:"chainId"`
	PaymentToken         string   `json:"paymentToken"`
	PaymentAmount        string   `json:"paymentAmount"`
	Metadata             Metadata `json:"metadata"`
}

// LineItem is a line item for a checkout session request for radom
type LineItem struct {
	ItemData  map[string]interface{} `json:"itemData"`
	ProductID string                 `json:"productId"`
}

// CheckoutSessionResponse is the structure received from submission of a checkout session
type CheckoutSessionResponse struct {
	CheckoutSessionID  string `json:"checkoutSessionId"`
	CheckoutSessionURL string `json:"checkoutSessionUrl"`
}

// CheckoutSessionRequest is the structure for submission of a checkout session
type CheckoutSessionRequest struct {
	AcceptedChains []int64                `json:"acceptedChains"`
	AcceptedTokens []AcceptedToken        `json:"acceptedTokens"`
	CancelURL      string                 `json:"cancelUrl"`
	Currency       string                 `json:"currency"`
	Customizations map[string]interface{} `json:"customizations"`
	ExpiresAt      int64                  `json:"expiresAt"` // in unix seconds
	LineItems      []LineItem             `json:"lineItems"`
	Metadata       Metadata               `json:"metadata"`
	SuccessURL     string                 `json:"successUrl"`
	sellerAddress  string                 `json:"sellerAddress"`
	Total          decimal.Decimal        `json:"total"`
}

// AcceptedToken is the structure for the accepted token in the checkout session
type AcceptedToken struct {
	ChainID      int64  `json:"chainId"`
	TokenAddress string `json:"tokenAddress"`
}

// Client is what a radom client should support
type Client interface {
	CreateCheckoutSession(*CheckoutSessionRequest) (*CheckoutSessionResponse, error)
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (Client, error) {
	serverEnvKey := "RADOM_SERVER"
	serverURL := os.Getenv(serverEnvKey)
	if len(serverURL) == 0 {
		return nil, errors.New(serverEnvKey + " was empty")
	}
	proxy := os.Getenv("HTTP_PROXY")
	client, err := clients.NewWithProxy("radom", serverURL, os.Getenv("RADOM_SECRET"), proxy)
	if err != nil {
		return nil, err
	}
	return NewClientWithPrometheus(&HTTPClient{client}, "radom_client"), err
}

// HTTPClient wraps http.Client
type HTTPClient struct {
	client *clients.SimpleHTTPClient
}

// CreateCheckoutSession will create a radom checkout session
func (hc *HTTPClient) CreateCheckoutSession(req *CheckoutSessionRequest) (*CheckoutSessionResponse, error) {
	return nil, errors.New("not implemented")
}

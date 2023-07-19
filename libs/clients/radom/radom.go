package radom

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/shopspring/decimal"

	"github.com/brave-intl/bat-go/libs/clients"
)

var (
	ErrInvalidMetadataKey = errors.New("failed to get key from radom metadata")
)

// CheckoutSessionRequest represents a request to create a checkout session.
type CheckoutSessionRequest struct {
	SuccessURL     string                 `json:"successUrl"`
	CancelURL      string                 `json:"cancelUrl"`
	Currency       string                 `json:"currency"`
	ExpiresAt      int64                  `json:"expiresAt"` // in unix seconds
	LineItems      []LineItem             `json:"lineItems"`
	Metadata       Metadata               `json:"metadata"`
	Customizations map[string]interface{} `json:"customizations"`
	Total          decimal.Decimal        `json:"total"`
	Gateway        Gateway                `json:"gateway"`
}

// Gateway provides access to managed services configurations
type Gateway struct {
	Managed Managed `json:"managed"`
}

// Managed is the Radom managed services configuration
type Managed struct {
	Methods []Method `json:"methods"`
}

// Method is a Radom payment method type
type Method struct {
	Network string `json:"network"`
	Token   string `json:"token"`
}

// CheckoutSessionResponse represents the result of submission of a checkout session.
type CheckoutSessionResponse struct {
	SessionID  string `json:"checkoutSessionId"`
	SessionURL string `json:"checkoutSessionUrl"`
}

// LineItem is a line item for a checkout session request.
type LineItem struct {
	ProductID string                 `json:"productId"`
	ItemData  map[string]interface{} `json:"itemData"`
}

// Metadata represents metaadata in a checkout session request.
type Metadata []KeyValue

// Get allows returns a value based on the key from the Radom metadata.
func (m Metadata) Get(key string) (string, error) {
	for _, v := range m {
		if subtle.ConstantTimeCompare([]byte(key), []byte(v.Key)) == 1 {
			return v.Value, nil
		}
	}

	return "", ErrInvalidMetadataKey
}

// KeyValue represents a key-value metadata pair.
type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// AutomatedEVMSubscripton defines an automated subscription
type AutomatedEVMSubscription struct {
	BuyerAddress                string `json:"buyerAddress"`
	SubscriptionContractAddress string `json:"subscriptionContractAddress"`
}

// Subscription is a radom subscription
type Subscription struct {
	AutomatedEVMSubscription AutomatedEVMSubscription `json:"automatedEVMSubscription"`
}

// NewSubscriptionData provides details about the new subscription
type NewSubscriptionData struct {
	SubscriptionID       string            `json:"subscriptionId"`
	Subscription         Subscription      `json:"subscriptionType"`
	Network              string            `json:"network"`
	Token                string            `json:"token"`
	Amount               decimal.Decimal   `json:"amount"`
	Currency             string            `json:"currency"`
	Period               string            `json:"period"`
	PeriodCustomDuration string            `json:"periodCustomDuration"`
	CreatedAt            *time.Time        `json:"createdAt"`
	Tags                 map[string]string `json:"tags"`
}

// RadomData is radom specific data attached to webhook calls
type RadomData struct {
	CheckoutSession CheckoutSession `json:"checkoutSession"`
}

// CheckoutSession describes a radom checkout session
type CheckoutSession struct {
	CheckoutSessionID string   `json:"checkoutSessionId"`
	Metadata          Metadata `json:"metadata"`
}

// ManagedRecurringPayment provides details about the recurring payment from webhook
type ManagedRecurringPayment struct {
	PaymentMethod Method          `json:"paymentMethod"`
	Amount        decimal.Decimal `json:"amount"`
}

// EventData encapsulates the webhook event
type EventData struct {
	ManagedRecurringPayment *ManagedRecurringPayment `json:"managedRecurringPayment"`
	NewSubscription         *NewSubscriptionData     `json:"newSubscription"`
}

// WebhookRequest represents a radom webhook submission
type WebhookRequest struct {
	EventType string    `json:"eventType"`
	EventData EventData `json:"eventData"`
	RadomData RadomData `json:"radomData"`
}

// Client communicates with Radom.
type Client struct {
	client *clients.SimpleHTTPClient
}

// New returns a ready to use Client.
func New(srvURL, secret, proxyAddr string) (*Client, error) {
	return newClient(srvURL, secret, proxyAddr)
}

func NewInstrumented(srvURL, secret, proxyAddr string) (*InstrumentedClient, error) {
	cl, err := newClient(srvURL, secret, proxyAddr)
	if err != nil {
		return nil, err
	}

	return newInstrucmentedClient("radom_client", cl), nil
}

func newClient(srvURL, secret, proxyAddr string) (*Client, error) {
	client, err := clients.NewWithProxy("radom", srvURL, secret, proxyAddr)
	if err != nil {
		return nil, err
	}

	return &Client{client: client}, nil
}

// CreateCheckoutSession creates a Radom checkout session.
func (c *Client) CreateCheckoutSession(
	ctx context.Context,
	req *CheckoutSessionRequest,
) (*CheckoutSessionResponse, error) {

	return nil, errors.New("not implemented")
}

package radom

import (
	"context"
	"net/http"

	"github.com/brave-intl/bat-go/libs/clients"
)

type Client struct {
	client *clients.SimpleHTTPClient
}

func New(srvURL, authToken, proxyAddr string) (*Client, error) {
	cl, err := clients.NewWithProxy("radom", srvURL, authToken, proxyAddr)
	if err != nil {
		return nil, err
	}

	return &Client{client: cl}, nil
}

type Gateway struct {
	Managed Managed `json:"managed"`
}

type Managed struct {
	Methods []Method `json:"methods"`
}

type Method struct {
	Network string `json:"network"`
	Token   string `json:"token"`
}

type LineItem struct {
	ProductID string `json:"productId"`
}

type Metadata struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type CheckoutSessionRequest struct {
	LineItems  []LineItem `json:"lineItems"`
	Gateway    Gateway    `json:"gateway"`
	SuccessURL string     `json:"successUrl"`
	CancelURL  string     `json:"cancelUrl"`
	Metadata   []Metadata `json:"metadata"`
	ExpiresAt  int64      `json:"expiresAt"` // in unix seconds
}

type CheckoutSessionResponse struct {
	SessionID  string `json:"checkoutSessionId"`
	SessionURL string `json:"checkoutSessionUrl"`
}

func (c *Client) CreateCheckoutSession(ctx context.Context, creq CheckoutSessionRequest) (CheckoutSessionResponse, error) {
	req, err := c.client.NewRequest(ctx, http.MethodPost, "/checkout_session", creq, nil)
	if err != nil {
		return CheckoutSessionResponse{}, err
	}

	var resp CheckoutSessionResponse
	if _, err := c.client.Do(ctx, req, &resp); err != nil {
		return CheckoutSessionResponse{}, err
	}

	return resp, nil
}

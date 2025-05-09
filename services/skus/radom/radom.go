package radom

import (
	"context"
	"net/http"
	"time"

	"github.com/brave-intl/bat-go/libs/clients"
)

type Client struct {
	client    *clients.SimpleHTTPClient
	authToken string
}

func New(srvURL, authToken string) (*Client, error) {
	cl, err := clients.New(srvURL, "")
	if err != nil {
		return nil, err
	}

	return &Client{client: cl, authToken: authToken}, nil
}

type Gateway struct {
	Managed Managed `json:"managed"`
}

type Managed struct {
	Methods []Method `json:"methods"`
}

type Method struct {
	Network            string  `json:"network"`
	Token              string  `json:"token"`
	DiscountPercentOff float64 `json:"discountPercentOff"`
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
	Gateway    *Gateway   `json:"gateway"`
	SuccessURL string     `json:"successUrl"`
	CancelURL  string     `json:"cancelUrl"`
	Metadata   []Metadata `json:"metadata"`
	ExpiresAt  int64      `json:"expiresAt"` // in unix seconds
}

type CheckoutSessionResponse struct {
	SessionID  string `json:"checkoutSessionId"`
	SessionURL string `json:"checkoutSessionUrl"`
}

func (c *Client) CreateCheckoutSession(ctx context.Context, creq *CheckoutSessionRequest) (CheckoutSessionResponse, error) {
	req, err := c.client.NewRequest(ctx, http.MethodPost, "/checkout_session", creq, nil)
	if err != nil {
		return CheckoutSessionResponse{}, err
	}

	req.Header.Add("Authorization", c.authToken)

	var resp CheckoutSessionResponse
	if _, err := c.client.Do(ctx, req, &resp); err != nil {
		return CheckoutSessionResponse{}, err
	}

	return resp, nil
}

func (c *Client) GetSubscription(ctx context.Context, subID string) (*SubscriptionResponse, error) {
	req, err := c.client.NewRequest(ctx, http.MethodGet, "/subscription/"+subID, nil, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", c.authToken)

	resp := &SubscriptionResponse{}
	if _, err := c.client.Do(ctx, req, resp); err != nil {
		return nil, err
	}

	return resp, nil
}

type SubscriptionResponse struct {
	ID                string    `json:"id"`
	NextBillingDateAt string    `json:"nextBillingDateAt"`
	Payments          []Payment `json:"payments"`
}

type Payment struct {
	Date string `json:"date"`
}

func (s *SubscriptionResponse) NextBillingDate() (time.Time, error) {
	nxtB, err := time.Parse(time.RFC3339, s.NextBillingDateAt)
	if err != nil {
		return time.Time{}, err
	}

	return nxtB.UTC(), nil
}

const ErrSubPaymentsEmpty = Error("radom: subscription payments empty")

func (s *SubscriptionResponse) LastPaid() (time.Time, error) {
	if len(s.Payments) <= 0 {
		return time.Time{}, ErrSubPaymentsEmpty
	}

	var paidAt time.Time

	for i := range s.Payments {
		pat, err := time.Parse(time.RFC3339, s.Payments[i].Date)
		if err != nil {
			return time.Time{}, err
		}

		if pat.After(paidAt) {
			paidAt = pat
		}
	}

	return paidAt.UTC(), nil
}

type Error string

func (e Error) Error() string {
	return string(e)
}

package ratios

import (
	"context"
	"time"

	"github.com/brave-intl/bat-go/utils/clients"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Client abstracts over the underlying client
type Client interface {
	FetchRate(ctx context.Context, id uuid.UUID) error
}

// HTTPClient wraps http.Client for interacting with the ledger server
type HTTPClient struct {
	clients.SimpleHTTPClient
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (*HTTPClient, error) {
	client, err := clients.New("RATIOS_SERVER", "RATIOS_TOKEN")
	if err != nil {
		return nil, err
	}
	return &HTTPClient{*client}, err
}

// RateResponse is the response received from ratios
type RateResponse struct {
	LastUpdated time.Time                  `json:"lastUpdated"`
	Payload     map[string]decimal.Decimal `json:"payload"`
}

// FetchRate fetches the rate of a currency to BAT
func (c *HTTPClient) FetchRate(ctx context.Context, base string, currency string) (*RateResponse, error) {
	req, err := c.NewRequest(ctx, "GET", "/v1/relative/"+base, map[string]string{
		"currency": currency,
	})
	if err != nil {
		return nil, err
	}

	var body RateResponse
	resp, err := c.Do(ctx, req, &body)

	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, nil
		}
		return nil, err
	}

	return &body, err
}

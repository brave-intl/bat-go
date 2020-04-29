package ratios

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/brave-intl/bat-go/utils/clients"
	appctx "github.com/brave-intl/bat-go/utils/context"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Client abstracts over the underlying client
type Client interface {
	FetchRate(ctx context.Context, id uuid.UUID) error
}

// HTTPClient wraps http.Client for interacting with the ledger server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
}

// NewWithContext returns a new HTTPClient, retrieving the base URL from the context
func NewWithContext(ctx context.Context) (*HTTPClient, error) {
	// get the server url from context
	serverURL, err := appctx.GetStringFromContext(ctx, appctx.RatiosServerCTXKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get RatiosServer from context: %w", err)
	}

	// get the server access token from context
	accessToken, err := appctx.GetStringFromContext(ctx, appctx.RatiosAccessTokenCTXKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get RatiosAccessToken from context: %w", err)
	}

	client, err := clients.New(serverURL, accessToken)
	if err != nil {
		return nil, err
	}
	return &HTTPClient{client}, err
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (*HTTPClient, error) {
	serverEnvKey := "RATIOS_SERVER"
	serverURL := os.Getenv("RATIOS_SERVER")
	if len(serverURL) == 0 {
		return nil, errors.New(serverEnvKey + " was empty")
	}
	client, err := clients.New(serverURL, os.Getenv("RATIOS_ACCESS_TOKEN"))
	if err != nil {
		return nil, err
	}
	return &HTTPClient{client}, err
}

// RateResponse is the response received from ratios
type RateResponse struct {
	LastUpdated time.Time                  `json:"lastUpdated"`
	Payload     map[string]decimal.Decimal `json:"payload"`
}

// FetchRate fetches the rate of a currency to BAT
func (c *HTTPClient) FetchRate(ctx context.Context, base string, currency string) (*RateResponse, error) {
	req, err := c.client.NewRequest(ctx, "GET", "/v1/relative/"+base, map[string]string{
		"currency": currency,
	})
	if err != nil {
		return nil, err
	}

	var body RateResponse
	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, err
	}

	return &body, nil
}

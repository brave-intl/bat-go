package ratios

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/brave-intl/bat-go/utils/clients"
	appctx "github.com/brave-intl/bat-go/utils/context"
	cache "github.com/patrickmn/go-cache"
	"github.com/shopspring/decimal"
)

// Client abstracts over the underlying client
type Client interface {
	FetchRate(ctx context.Context, base string, currency string) (*RateResponse, error)
}

// HTTPClient wraps http.Client for interacting with the ledger server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
	cache  *cache.Cache
}

// NewWithContext returns a new HTTPClient, retrieving the base URL from the context
func NewWithContext(ctx context.Context) (Client, error) {
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

	// get default timeout and purge from context
	expires, err := appctx.GetDurationFromContext(ctx, appctx.RatiosCacheExpiryDurationCTXKey)
	if err != nil {
		expires = 5 * time.Second
	}

	// get default purge and purge from context
	purge, err := appctx.GetDurationFromContext(ctx, appctx.RatiosCachePurgeDurationCTXKey)
	if err != nil {
		purge = 1 * time.Minute
	}

	return NewClientWithPrometheus(
		&HTTPClient{
			client: client,
			cache:  cache.New(expires, purge),
		}, "ratios_context_client"), nil
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (Client, error) {
	serverEnvKey := "RATIOS_SERVER"
	serverURL := os.Getenv("RATIOS_SERVER")
	if len(serverURL) == 0 {
		return nil, errors.New(serverEnvKey + " was empty")
	}
	client, err := clients.New(serverURL, os.Getenv("RATIOS_ACCESS_TOKEN"))
	if err != nil {
		return nil, err
	}
	return NewClientWithPrometheus(
		&HTTPClient{
			client: client,
			cache:  cache.New(5*time.Second, 1*time.Minute),
		}, "ratios_client"), err
}

// RateResponse is the response received from ratios
type RateResponse struct {
	LastUpdated time.Time                  `json:"lastUpdated"`
	Payload     map[string]decimal.Decimal `json:"payload"`
}

// FetchRate fetches the rate of a currency to BAT
func (c *HTTPClient) FetchRate(ctx context.Context, base string, currency string) (*RateResponse, error) {
	var cacheKey = fmt.Sprintf("%s_%s", base, currency)
	// check cache for this rate
	if rate, found := c.cache.Get(cacheKey); found {
		return rate.(*RateResponse), nil
	}

	url := fmt.Sprintf("/v1/relative/%s", base)
	req, err := c.client.NewRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = fmt.Sprintf("currency=%s", currency)

	var body RateResponse
	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, err
	}

	c.cache.Set(cacheKey, &body, cache.DefaultExpiration)

	return &body, nil
}

package ratios

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/clients/coingecko"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/google/go-querystring/query"
	cache "github.com/patrickmn/go-cache"
	"github.com/shopspring/decimal"
)

// Client abstracts over the underlying client
type Client interface {
	FetchRate(ctx context.Context, base string, currency string) (*RateResponse, error)
}

// HTTPClient wraps http.Client for interacting with the ratios server
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
	serverEnvKey := "RATIOS_SERVICE"
	serverURL := os.Getenv(serverEnvKey)
	if len(serverURL) == 0 {
		return nil, errors.New(serverEnvKey + " was empty")
	}
	client, err := clients.New(serverURL, os.Getenv("RATIOS_TOKEN"))
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

// FetchOptions options for fetching rates from ratios
type FetchOptions struct {
	Currency string `url:"currency,omitempty"`
}

// GenerateQueryString - implement the QueryStringBody interface
func (fo *FetchOptions) GenerateQueryString() (url.Values, error) {
	return query.Values(fo)
}

// RelativeResponse - the relative response structure
type RelativeResponse struct {
	Payload     coingecko.SimplePriceResponse `json:"payload"`
	LastUpdated time.Time                     `json:"lastUpdated"`
}

// FetchRate fetches the rate of a currency to BAT
func (c *HTTPClient) FetchRate(ctx context.Context, base string, currency string) (*RateResponse, error) {
	// normalize base and currency to lowercase
	base = strings.ToLower(base)
	currency = strings.ToLower(currency)

	var cacheKey = fmt.Sprintf("%s_%s", base, currency)
	// check cache for this rate
	if rate, found := c.cache.Get(cacheKey); found {
		return rate.(*RateResponse), nil
	}

	url := fmt.Sprintf("/v2/relative/provider/coingecko/%s/%s/1d", base, currency)
	req, err := c.client.NewRequest(ctx, "GET", url, nil, nil)
	if err != nil {
		return nil, err
	}

	var body RelativeResponse
	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, err
	}

	resp := RateResponse{
		Payload:     body.Payload[base],
		LastUpdated: time.Now(),
	}

	c.cache.Set(cacheKey, &resp, cache.DefaultExpiration)

	return &resp, nil
}

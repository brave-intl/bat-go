package coingecko

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/brave-intl/bat-go/utils/clients"
	"github.com/brave-intl/bat-go/utils/closers"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/gomodule/redigo/redis"
	"github.com/google/go-querystring/query"
	"github.com/shopspring/decimal"
)

// Client abstracts over the underlying client
type Client interface {
	FetchSimplePrice(ctx context.Context, ids string, vsCurrencies string, include24hrChange bool) (*SimplePriceResponse, error)
	FetchCoinList(ctx context.Context, includePlatform bool) (*CoinListResponse, error)
	FetchSupportedVsCurrencies(ctx context.Context) (*SupportedVsCurrenciesResponse, error)
	FetchMarketChart(ctx context.Context, id string, vsCurrency string, days float32) (*MarketChartResponse, time.Time, error)
}

// HTTPClient wraps http.Client for interacting with the coingecko server
type HTTPClient struct {
	baseParams
	client *clients.SimpleHTTPClient
	redis  *redis.Pool
}

// NewWithContext returns a new HTTPClient, retrieving the base URL from the context
func NewWithContext(ctx context.Context, redis *redis.Pool) (Client, error) {
	// get the server url from context
	serverURL, err := appctx.GetStringFromContext(ctx, appctx.CoingeckoServerCTXKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get CoingeckoServer from context: %w", err)
	}

	// get the server access token from context
	accessToken, err := appctx.GetStringFromContext(ctx, appctx.CoingeckoAccessTokenCTXKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get CoingeckoAccessToken from context: %w", err)
	}

	client, err := clients.NewWithHTTPClient(serverURL, "", &http.Client{
		Timeout: time.Second * 30,
	})
	if err != nil {
		return nil, err
	}

	return NewClientWithPrometheus(
		&HTTPClient{
			baseParams: baseParams{
				APIKey: accessToken,
			},
			client: client,
			redis:  redis,
		}, "coingecko_context_client"), nil
}

func (c *HTTPClient) cacheKey(ctx context.Context, path string, body clients.QueryStringBody) (string, error) {
	qs, err := body.GenerateQueryString()
	if err != nil {
		return "", err
	}

	// redact API key
	qs.Del("x_cg_pro_api_key")

	return c.client.BaseURL.ResolveReference(&url.URL{
		Path:     path,
		RawQuery: qs.Encode(),
	}).String(), nil
}

type cacheEntry struct {
	Payload     string    `json:"payload"`
	LastUpdated time.Time `json:"lastUpdated"`
}

// baseParams that must be included with every request
type baseParams struct {
	APIKey string `url:"x_cg_pro_api_key"`
}

// simplePriceParams for fetching prices
type simplePriceParams struct {
	baseParams
	Ids               string `url:"ids"`
	VsCurrencies      string `url:"vs_currencies"`
	Include24hrChange bool   `url:"include_24hr_change,omitempty"`
}

// GenerateQueryString - implement the QueryStringBody interface
func (p *simplePriceParams) GenerateQueryString() (url.Values, error) {
	return query.Values(p)
}

// SimplePriceResponse is the response received from coingecko
type SimplePriceResponse map[string]map[string]decimal.Decimal

// FetchSimplePrice fetches the rate of a currency to BAT
func (c *HTTPClient) FetchSimplePrice(ctx context.Context, ids string, vsCurrencies string, include24hrChange bool) (*SimplePriceResponse, error) {
	req, err := c.client.NewRequest(ctx, "GET", "/api/v3/simple/price", nil, &simplePriceParams{
		baseParams:        c.baseParams,
		Ids:               ids,
		VsCurrencies:      vsCurrencies,
		Include24hrChange: include24hrChange,
	})
	if err != nil {
		return nil, err
	}

	var body SimplePriceResponse
	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, err
	}

	return &body, nil
}

// marketChartParams for fetching market chart
type marketChartParams struct {
	baseParams
	ID         string  `url:"id"`
	VsCurrency string  `url:"vs_currency"`
	Days       float32 `url:"days,omitempty"`
}

// GenerateQueryString - implement the QueryStringBody interface
func (p *marketChartParams) GenerateQueryString() (url.Values, error) {
	return query.Values(p)
}

// MarketChartResponse is the response received from coingecko
type MarketChartResponse struct {
	Prices       [][]decimal.Decimal `json:"prices"`
	MarketCaps   [][]decimal.Decimal `json:"market_caps"`
	TotalVolumes [][]decimal.Decimal `json:"total_volumes"`
}

// FetchMarketChart fetches the history rate of a currency to BAT
func (c *HTTPClient) FetchMarketChart(ctx context.Context, id string, vsCurrency string, days float32) (*MarketChartResponse, time.Time, error) {
	updated := time.Now()

	url := fmt.Sprintf("/api/v3/coins/%s/market_chart", id)
	params := &marketChartParams{
		baseParams: c.baseParams,
		ID:         id,
		VsCurrency: vsCurrency,
		Days:       days,
	}
	cacheKey, err := c.cacheKey(ctx, url, params)
	if err != nil {
		return nil, updated, err
	}

	conn := c.redis.Get()
	defer closers.Log(ctx, conn)

	var body MarketChartResponse
	var entry cacheEntry
	entryBytes, err := redis.Bytes(conn.Do("GET", cacheKey))
	if err == nil {
		err = json.Unmarshal(entryBytes, &entry)
		if err != nil {
			return nil, updated, err
		}
		err = json.Unmarshal([]byte(entry.Payload), &body)
		if err != nil {
			return nil, updated, err
		}

		// 1h chart is cached for 2.5m
		// 1d chart is cached for 1 hour
		// 1w chart is cached for 7 hours
		// etc
		if time.Since(entry.LastUpdated).Hours() < float64(days) {
			return &body, entry.LastUpdated, err
		}
	}

	req, err := c.client.NewRequest(ctx, "GET", url, nil, params)
	if err != nil {
		return nil, updated, err
	}

	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		// attempt to use cache response on error if exists
		if len(entry.Payload) > 0 {
			return &body, entry.LastUpdated, nil
		}
		return nil, updated, err
	}

	bodyBytes, err := json.Marshal(&body)
	if err != nil {
		return nil, updated, err
	}
	entryBytes, err = json.Marshal(&cacheEntry{Payload: string(bodyBytes), LastUpdated: updated})
	if err != nil {
		return nil, updated, err
	}
	_, err = conn.Do("SET", cacheKey, entryBytes)
	if err != nil {
		return nil, updated, err
	}

	return &body, updated, nil
}

type coinListParams struct {
	baseParams
	IncludePlatform bool `url:"include_platform"`
}

// GenerateQueryString - implement the QueryStringBody interface
func (p *coinListParams) GenerateQueryString() (url.Values, error) {
	return query.Values(p)
}

// CoinInfoPlatform - platform coin info
type CoinInfoPlatform struct {
	Ethereum string `json:"ethereum,omitempty"`
}

// CoinInfo - info about coin
type CoinInfo struct {
	ID        string           `json:"id"`
	Symbol    string           `json:"symbol"`
	Name      string           `json:"name"`
	Platforms CoinInfoPlatform `json:"platforms,omitempty"`
}

// CoinListResponse is the response received from coingecko
type CoinListResponse []CoinInfo

// FetchCoinList fetches the list of supported coins
func (c *HTTPClient) FetchCoinList(ctx context.Context, includePlatform bool) (*CoinListResponse, error) {
	req, err := c.client.NewRequest(ctx, "GET", "/api/v3/coins/list", nil, &coinListParams{
		baseParams:      c.baseParams,
		IncludePlatform: includePlatform,
	})
	if err != nil {
		return nil, err
	}

	var body CoinListResponse
	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, err
	}

	return &body, nil
}

type supportedVsCurrenciesParams struct {
	baseParams
}

// GenerateQueryString - implement the QueryStringBody interface
func (p *supportedVsCurrenciesParams) GenerateQueryString() (url.Values, error) {
	return query.Values(p)
}

// SupportedVsCurrenciesResponse is the response received from coingecko
type SupportedVsCurrenciesResponse []string

// FetchSupportedVsCurrencies fetches the list of supported vs currencies
func (c *HTTPClient) FetchSupportedVsCurrencies(ctx context.Context) (*SupportedVsCurrenciesResponse, error) {
	req, err := c.client.NewRequest(ctx, "GET", "/api/v3/simple/supported_vs_currencies", nil, &supportedVsCurrenciesParams{
		baseParams: c.baseParams,
	})
	if err != nil {
		return nil, err
	}

	var body SupportedVsCurrenciesResponse
	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, err
	}

	return &body, nil
}

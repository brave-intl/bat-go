package coingecko

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/brave-intl/bat-go/utils/clients"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/google/go-querystring/query"
	cache "github.com/patrickmn/go-cache"
	"github.com/shopspring/decimal"
)

// Client abstracts over the underlying client
type Client interface {
	FetchSimplePrice(ctx context.Context, ids string, vsCurrencies string, include24hrChange bool) (*SimplePriceResponse, error)
	FetchCoinList(ctx context.Context, includePlatform bool) (*CoinListResponse, error)
	FetchSupportedVsCurrencies(ctx context.Context) (*SupportedVsCurrenciesResponse, error)
	FetchMarketChart(ctx context.Context, id string, vsCurrency string, days float32) (*MarketChartResponse, error)
}

// HTTPClient wraps http.Client for interacting with the coingecko server
type HTTPClient struct {
	baseParams
	client *clients.SimpleHTTPClient
	cache  *cache.Cache
}

// NewWithContext returns a new HTTPClient, retrieving the base URL from the context
func NewWithContext(ctx context.Context) (Client, error) {
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

	client, err := clients.NewWithHttpClient(serverURL, "", &http.Client{
		Timeout: time.Second * 30,
	})
	if err != nil {
		return nil, err
	}

	// get default timeout and purge from context
	//expires, err := appctx.GetDurationFromContext(ctx, appctx.RatiosCacheExpiryDurationCTXKey)
	//if err != nil {
	//expires = 5 * time.Second
	//}

	// get default purge and purge from context
	//purge, err := appctx.GetDurationFromContext(ctx, appctx.RatiosCachePurgeDurationCTXKey)
	//if err != nil {
	//purge = 1 * time.Minute
	//}

	return NewClientWithPrometheus(
		&HTTPClient{
			baseParams: baseParams{
				ApiKey: accessToken,
			},
			client: client,
			cache:  cache.New(cache.NoExpiration, cache.NoExpiration),
		}, "coingecko_context_client"), nil
}

// baseParams that must be included with every request
type baseParams struct {
	ApiKey string `url:"x_cg_pro_api_key"`
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

// FetchRate fetches the rate of a currency to BAT
func (c *HTTPClient) FetchSimplePrice(ctx context.Context, ids string, vsCurrencies string, include24hrChange bool) (*SimplePriceResponse, error) {
	//var cacheKey = fmt.Sprintf("%s_%s", base, currency)
	// check cache for this rate
	//if rate, found := c.cache.Get(cacheKey); found {
	//return rate.(*RateResponse), nil
	//}

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

	//c.cache.Set(cacheKey, &body, cache.DefaultExpiration)

	return &body, nil
}

// marketChartParams for fetching market chart
type marketChartParams struct {
	baseParams
	Id         string  `url:"id"`
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

// FetchRate fetches the rate of a currency to BAT
func (c *HTTPClient) FetchMarketChart(ctx context.Context, id string, vsCurrency string, days float32) (*MarketChartResponse, error) {
	//var cacheKey = fmt.Sprintf("%s_%s", base, currency)
	// check cache for this rate
	//if rate, found := c.cache.Get(cacheKey); found {
	//return rate.(*RateResponse), nil
	//}

	url := fmt.Sprintf("/api/v3/coins/%s/market_chart", id)
	req, err := c.client.NewRequest(ctx, "GET", url, nil, &marketChartParams{
		baseParams: c.baseParams,
		Id:         id,
		VsCurrency: vsCurrency,
		Days:       days,
	})
	if err != nil {
		return nil, err
	}

	var body MarketChartResponse
	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, err
	}

	//c.cache.Set(cacheKey, &body, cache.DefaultExpiration)

	return &body, nil
}

type coinListParams struct {
	baseParams
	IncludePlatform bool `url:"include_platform"`
}

// GenerateQueryString - implement the QueryStringBody interface
func (p *coinListParams) GenerateQueryString() (url.Values, error) {
	return query.Values(p)
}

type CoinInfoPlatform struct {
	Ethereum string `json:"ethereum,omitempty"`
}

type CoinInfo struct {
	Id        string           `json:"id"`
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

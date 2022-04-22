package etherscan

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
)

const (
	gasOracleCacheTTLHours = 1 // How long we consider Redis cached FetchCoinMarkets responses to be valid
)

// Client abstracts over the underlying client
type Client interface {
	FetchGasOracle(ctx context.Context) (*GasOracleResponse, time.Time, error)
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
	serverURL, err := appctx.GetStringFromContext(ctx, appctx.EtherscanURICTXKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get EtherscanServer from context: %w", err)
	}

	// get the server access token from context
	accessToken, err := appctx.GetStringFromContext(ctx, appctx.EtherscanTokenCTXKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get EtherscanToken from context: %w", err)
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
		}, "etherscan_context_client"), nil
}

func (c *HTTPClient) cacheKey(ctx context.Context, path string, body clients.QueryStringBody) (string, error) {
	qs, err := body.GenerateQueryString()
	if err != nil {
		return "", err
	}

	// redact API key
	qs.Del("apikey")

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
	APIKey string `url:"apikey"`
}

// gasOracleParams for fetching gas oracle
type gasOracleParams struct {
	baseParams
	Module string `url:"gastracker"`
	Action string `url:"gasoracle"`
}

// GenerateQueryString - implement the QueryStringBody interface
func (p *gasOracleParams) GenerateQueryString() (url.Values, error) {
	return query.Values(p)
}

// GasOracleResponse is the response received from coingecko
type GasOracleResponse map[string]interface{}

// FetchGasOracle fetches the market data for the top coins
func (c *HTTPClient) FetchGasOracle(
	ctx context.Context,
) (*GasOracleResponse, time.Time, error) {

	updated := time.Now()
	url := "/api"
	params := &gasOracleParams{
		baseParams: c.baseParams,
		Module:     "gastracker",
		Action:     "gasoracle",
	}

	cacheKey, err := c.cacheKey(ctx, url, params)
	if err != nil {
		return nil, updated, err
	}

	conn := c.redis.Get()
	defer closers.Log(ctx, conn)

	var body GasOracleResponse
	var entry cacheEntry
	entryBytes, err := redis.Bytes(conn.Do("GET", cacheKey))

	// Check cache first before making request to Etherscan
	if err == nil {
		err = json.Unmarshal(entryBytes, &entry)
		if err != nil {
			return nil, updated, err
		}

		err = json.Unmarshal([]byte(entry.Payload), &body)
		if err != nil {
			return nil, updated, err
		}

		// Check if cache is still fresh
		if time.Since(entry.LastUpdated).Hours() < float64(gasOracleCacheTTLHours) {
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

	// Update the cache
	entryBytes, err = json.Marshal(&cacheEntry{Payload: string(bodyBytes), LastUpdated: updated})
	if err != nil {
		return nil, updated, err
	}

	_, err = conn.Do("SET", cacheKey, entryBytes)
	if err != nil {
		return nil, updated, err
	}

	// Apply the limit only after caching the all the results
	return &body, updated, nil
}

package coingeckoAssets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/brave-intl/bat-go/utils/clients"
	"github.com/brave-intl/bat-go/utils/closers"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/gomodule/redigo/redis"
)

const (
	imageAssetsCacheTTLHours = 24
)

// Client abstracts over the underlying client
type Client interface {
	FetchImageAsset(ctx context.Context, imageID string, size string, imageFile string) (*ImageAssetResponseBundle, time.Time, error)
}

// HTTPClient wraps http.Client for interacting with the coingecko server
type HTTPClient struct {
	// baseParams
	client *clients.SimpleHTTPClient
	redis  *redis.Pool
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

// NewWithContext returns a new HTTPClient, retrieving the base URL from the context
func NewWithContext(ctx context.Context, redis *redis.Pool) (Client, error) {
	serverURL, err := appctx.GetStringFromContext(ctx, appctx.CoingeckoAssetsServerCTXKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get CoingeckoAssetsServer from context: %w", err)
	}

	client, err := clients.NewWithHTTPClient(serverURL, "", &http.Client{
		Timeout: time.Second * 30,
	})
	if err != nil {
		return nil, err
	}

	return NewClientWithPrometheus(
		&HTTPClient{
			client: client,
			redis:  redis,
		}, "coingecko_context_assets_client"), nil
}

// ImageAssetResponseBundle contains the payload (the image data) as well as the content
// type header value so it can be set when responding to the client
// This is the Payload of the cacheEntry stored in Redis
type ImageAssetResponseBundle struct {
	ImageData   []byte
	ContentType string
}

type imageAssetParams struct{}

// GenerateQueryString - implement the QueryStringBody interface
func (p *imageAssetParams) GenerateQueryString() (url.Values, error) {
	return url.Values{}, nil
}

// FetchImageAsset fetches the image asset from coingecko
func (c *HTTPClient) FetchImageAsset(
	ctx context.Context,
	imageID string,
	size string,
	imageFile string,
) (*ImageAssetResponseBundle, time.Time, error) {
	updated := time.Now()
	url := fmt.Sprintf("/coins/images/%s/%s/%s", imageID, size, imageFile)
	params := &imageAssetParams{}
	cacheKey, err := c.cacheKey(ctx, url, params)
	if err != nil {
		return nil, updated, err
	}

	conn := c.redis.Get()
	defer closers.Log(ctx, conn)

	var responseBundle ImageAssetResponseBundle
	var entry cacheEntry

	// Check cache first before making request to Coingecko
	entryBytes, err := redis.Bytes(conn.Do("GET", cacheKey))
	if err == nil {
		err = json.Unmarshal(entryBytes, &entry)
		if err != nil {
			return nil, updated, err
		}

		err = json.Unmarshal([]byte(entry.Payload), &responseBundle)
		if err != nil {
			return nil, updated, err
		}

		// Check if cache is still fresh and use it if it is
		if time.Since(entry.LastUpdated).Hours() < float64(imageAssetsCacheTTLHours) {
			return &responseBundle, entry.LastUpdated, err
		}
	}

	// Otherwise, make the request to coingecko
	req, err := c.client.NewRequest(ctx, "GET", url, nil, params)
	if err != nil {
		return nil, updated, err
	}

	// c.client.Do expects a JSON response and raises an error
	// if it's not JSON. As a result, nothing is read into
	// responseBody.
	//
	// We have to make an exception for ErrUnableToDecode for this case
	// and then manually read the responseBody after
	var responseBody []byte
	resp, err := c.client.Do(ctx, req, &responseBody)
	if err != nil && err.Error() != clients.ErrUnableToDecode {
		// Use the out of date data from cache if request for new data fails
		if len(entry.Payload) > 0 {
			return &responseBundle, entry.LastUpdated, nil
		}
		return nil, updated, err
	}

	responseBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, updated, err
	}

	responseBundle.ImageData = responseBody
	responseBundle.ContentType = resp.Header.Get("content-type")

	responseBundleBytes, err := json.Marshal(&responseBundle)
	if err != nil {
		return nil, updated, err
	}

	// Update the cache
	entryBytes, err = json.Marshal(&cacheEntry{Payload: string(responseBundleBytes), LastUpdated: updated})
	if err != nil {
		return nil, updated, err
	}

	_, err = conn.Do("SET", cacheKey, entryBytes)
	if err != nil {
		return nil, updated, err
	}

	return &responseBundle, updated, nil
}

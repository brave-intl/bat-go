package coingeckoAssets

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/brave-intl/bat-go/utils/clients"
	appctx "github.com/brave-intl/bat-go/utils/context"
)

// Client abstracts over the underlying client
type Client interface {
	FetchImageAsset(ctx context.Context, imageID string, size string, imageFile string) (*ImageAssetResponseBundle, error)
}

// HTTPClient wraps http.Client for interacting with the coingecko server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
}

// NewWithContext returns a new HTTPClient, retrieving the base URL from the context
func NewWithContext(ctx context.Context) (Client, error) {
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
) (*ImageAssetResponseBundle, error) {
	url := fmt.Sprintf("/coins/images/%s/%s/%s", imageID, size, imageFile)
	params := &imageAssetParams{}
	var responseBundle ImageAssetResponseBundle

	req, err := c.client.NewRequest(ctx, "GET", url, nil, params)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	responseBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	responseBundle.ImageData = responseBody
	responseBundle.ContentType = resp.Header.Get("content-type")

	return &responseBundle, nil
}

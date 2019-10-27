package ads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/rs/zerolog/log"
)

// Client abstracts over the underlying client
type Client interface {
	GetAdsCountries(ctx context.Context) (map[string]string, error)
}

// HTTPClient wraps http.Client for interacting with the ledger server
type HTTPClient struct {
	BaseURL *url.URL

	client *http.Client
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (*HTTPClient, error) {
	serverURL := os.Getenv("ADS_SERVER")

	if len(serverURL) == 0 {
		return nil, errors.New("ADS_SERVER was empty")
	}

	baseURL, err := url.Parse(serverURL)

	if err != nil {
		return nil, err
	}

	return &HTTPClient{
		BaseURL: baseURL,
		client: &http.Client{
			Timeout: time.Second * 10,
		},
	}, nil
}

func (c *HTTPClient) newRequest(ctx context.Context, method, path string, body interface{}) (*http.Request, error) {
	url := c.BaseURL.ResolveReference(&url.URL{Path: path})

	var buf io.ReadWriter
	if body != nil {
		buf = new(bytes.Buffer)
		err := json.NewEncoder(buf).Encode(body)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, url.String(), buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")

	if body != nil {
		req.Header.Add("content-type", "application/json")
	}

	logger := log.Ctx(ctx)

	dump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		panic(err)
	}

	logger.Debug().Str("type", "http.Request").Msg(string(dump))

	return req, err
}

func (c *HTTPClient) do(req *http.Request, v interface{}) (*http.Response, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closers.Panic(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		if v != nil {
			err = json.NewDecoder(resp.Body).Decode(v)
			if err != nil {
				return nil, err
			}
		}
		return resp, nil
	}
	return resp, fmt.Errorf("Request error: %d", resp.StatusCode)
}

// CountryInfo contains info about a country where ads is enabled
type CountryInfo struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// GetAdsCountries retrieves info about countries where ads are enabled
func (c *HTTPClient) GetAdsCountries(ctx context.Context) (map[string]string, error) {
	req, err := c.newRequest(ctx, "GET", "v1/geoCode", nil)
	if err != nil {
		return map[string]string{}, err
	}

	var resp []CountryInfo
	_, err = c.do(req, &resp)

	countryMap := make(map[string]string, len(resp))

	for _, country := range resp {
		countryMap[country.Code] = country.Name
	}

	return countryMap, err
}

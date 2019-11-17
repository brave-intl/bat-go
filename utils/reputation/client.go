package reputation

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
	uuid "github.com/satori/go.uuid"
)

// Client abstracts over the underlying client
type Client interface {
	IsWalletReputable(ctx context.Context, id uuid.UUID) (bool, error)
}

// HTTPClient wraps http.Client for interacting with the reputation server
type HTTPClient struct {
	BaseURL   *url.URL
	AuthToken string

	client *http.Client
}

// New returns a new HTTPClient, retrieving the base URL from the
// environment
func New() (*HTTPClient, error) {
	serverURL := os.Getenv("REPUTATION_SERVER")

	if len(serverURL) == 0 && os.Getenv("ENV") != "local" {
		return nil, errors.New("REPUTATION_SERVER is missing in production environment")
	}

	baseURL, err := url.Parse(serverURL)

	if err != nil {
		return nil, err
	}

	return &HTTPClient{
		BaseURL:   baseURL,
		AuthToken: os.Getenv("REPUTATION_TOKEN"),
		client: &http.Client{
			Timeout: time.Second * 10,
		},
	}, nil
}

func (c *HTTPClient) newRequest(
	ctx context.Context,
	method,
	path string,
	body interface{},
) (*http.Request, error) {
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

	req.Header.Set("authorization", "Bearer "+c.AuthToken)

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

// IsWalletReputableResponse is what the reputation server
// will send back when we ask if a wallet is reputable
type IsWalletReputableResponse struct {
	IsReputable bool `json:"isReputable"`
}

// IsWalletReputable makes the request to the reputation server
// and reutrns whether a paymentId has enough reputation
// to claim a grant
func (c *HTTPClient) IsWalletReputable(
	ctx context.Context,
	paymentID uuid.UUID,
) (bool, error) {
	req, err := c.newRequest(
		ctx,
		"GET", "v1/reputation/"+paymentID.String(),
		nil,
	)
	if err != nil {
		return false, err
	}

	var resp IsWalletReputableResponse
	_, err = c.do(req, &resp)
	if err != nil {
		return false, err
	}

	return resp.IsReputable, nil
}

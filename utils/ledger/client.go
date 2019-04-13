package ledger

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

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
)

// Client abstracts over the underlying client
type Client interface {
	GetWallet(ctx context.Context, id uuid.UUID) (*wallet.Info, error)
}

// HTTPClient wraps http.Client for interacting with the ledger server
type HTTPClient struct {
	BaseURL   *url.URL
	AuthToken string

	client *http.Client
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (*HTTPClient, error) {
	serverURL := os.Getenv("LEDGER_SERVER")

	if len(serverURL) == 0 {
		return nil, errors.New("LEDGER_SERVER was empty")
	}

	baseURL, err := url.Parse(serverURL)

	if err != nil {
		return nil, err
	}

	return &HTTPClient{
		BaseURL:   baseURL,
		AuthToken: os.Getenv("LEDGER_TOKEN"),
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

	req.Header.Set("authorization", "Bearer: "+c.AuthToken)

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

// WalletAddresses contains the wallet addresses
type WalletAddresses struct {
	ProviderID uuid.UUID `json:"CARD_ID"`
}

// WalletResponse contains information about the ledger wallet
type WalletResponse struct {
	Addresses   WalletAddresses          `json:"addresses"`
	AltCurrency *altcurrency.AltCurrency `json:"altcurrency"`
	PublicKey   string                   `json:"httpSigningPubKey"`
}

// GetWallet retrieves wallet information
func (c *HTTPClient) GetWallet(ctx context.Context, id uuid.UUID) (*wallet.Info, error) {
	req, err := c.newRequest(ctx, "GET", "v2/wallet/"+id.String(), nil)
	if err != nil {
		return nil, err
	}

	var resp WalletResponse
	_, err = c.do(req, &resp)

	info := wallet.Info{
		ID:          id.String(),
		Provider:    "uphold",
		ProviderID:  resp.Addresses.ProviderID.String(),
		AltCurrency: resp.AltCurrency,
		PublicKey:   resp.PublicKey,
		LastBalance: nil,
	}

	return &info, err
}

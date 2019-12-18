package balance

import (
	"context"

	"github.com/brave-intl/bat-go/utils/clients"
	uuid "github.com/satori/go.uuid"
)

// Client abstracts over the underlying client
type Client interface {
	InvalidateBalance(ctx context.Context, id uuid.UUID) error
}

// HTTPClient wraps http.Client for interacting with the ledger server
type HTTPClient struct {
	clients.SimpleHTTPClient
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (*HTTPClient, error) {
	client, err := clients.New("BALANCE_SERVER", "BALANCE_TOKEN")
	if err != nil {
		return nil, err
	}
	return &HTTPClient{*client}, err
}

// InvalidateBalance invalidates the cached value on balance
func (c *HTTPClient) InvalidateBalance(ctx context.Context, id uuid.UUID) error {
	req, err := c.NewRequest(ctx, "DELETE", "v2/wallet/"+id.String()+"/balance", nil)
	if err != nil {
		return err
	}

	_, err = c.Do(ctx, req, nil)

	if err != nil {
		return err
	}

	return nil
}

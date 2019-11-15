package balance

import (
	"context"
	"errors"
	"os"

	"github.com/brave-intl/bat-go/utils/clients"
	uuid "github.com/satori/go.uuid"
)

// Client abstracts over the underlying client
type Client interface {
	InvalidateBalance(ctx context.Context, id uuid.UUID) error
}

// HTTPClient wraps http.Client for interacting with the ledger server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (*HTTPClient, error) {
	serverEnvKey := "BALANCE_SERVER"
	serverURL := os.Getenv(serverEnvKey)
	if len(serverEnvKey) == 0 {
		return nil, errors.New(serverEnvKey + " was empty")
	}
	client, err := clients.New(serverURL, os.Getenv("BALANCE_TOKEN"))
	if err != nil {
		return nil, err
	}
	return &HTTPClient{client}, err
}

// InvalidateBalance invalidates the cached value on balance
func (c *HTTPClient) InvalidateBalance(ctx context.Context, id uuid.UUID) error {
	req, err := c.client.NewRequest(ctx, "DELETE", "v2/wallet/"+id.String()+"/balance", nil)
	if err != nil {
		return err
	}

	_, err = c.client.Do(ctx, req, nil)

	if err != nil {
		return err
	}

	return nil
}

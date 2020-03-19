package reputation

import (
	"context"
	"errors"
	"os"

	"github.com/brave-intl/bat-go/utils/clients"
	uuid "github.com/satori/go.uuid"
)

// Client abstracts over the underlying client
type Client interface {
	IsWalletReputable(ctx context.Context, id uuid.UUID, platform string) (bool, error)
}

// HTTPClient wraps http.Client for interacting with the reputation server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
}

// New returns a new HTTPClient, retrieving the base URL from the
// environment
func New() (*HTTPClient, error) {
	serverEnvKey := "REPUTATION_SERVER"
	serverURL := os.Getenv(serverEnvKey)

	if len(serverURL) == 0 {
		if os.Getenv("ENV") != "local" {
			return nil, errors.New("REPUTATION_SERVER is missing in production environment")
		}
		return nil, errors.New(serverEnvKey + " was empty")
	}

	client, err := clients.New(serverURL, os.Getenv("REPUTATION_TOKEN"))
	if err != nil {
		return nil, err
	}

	return &HTTPClient{client}, nil
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
	platform string,
) (bool, error) {
	req, err := c.client.NewRequest(
		ctx,
		"GET",
		"v1/reputation/"+paymentID.String(),
		nil,
	)
	if err != nil {
		return false, err
	}

	var resp IsWalletReputableResponse
	_, err = c.client.Do(ctx, req, &resp)
	if err != nil {
		return false, err
	}

	return resp.IsReputable, nil
}

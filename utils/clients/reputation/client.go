package reputation

import (
	"context"

	"github.com/brave-intl/bat-go/utils/clients"
	uuid "github.com/satori/go.uuid"
)

// Client abstracts over the underlying client
type Client interface {
	IsWalletReputable(ctx context.Context, id uuid.UUID, platform string) (bool, error)
}

// HTTPClient wraps http.Client for interacting with the ledger server
type HTTPClient struct {
	clients.SimpleHTTPClient
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (*HTTPClient, error) {
	client, err := clients.New("REPUTATION_SERVER", "REPUTATION_TOKEN")
	if err != nil {
		return &HTTPClient{*client}, err
	}
	return nil, err
}

// IsWalletReputableResponse is what the reputation server
// will send back when we ask if a wallet is reputable
type IsWalletReputableResponse struct {
	IsReputable bool `json:"isReputable"`
}

// IsWalletReputable makes the request to the reputation server
// and reutrns whether a walletID has enough reputation
// to claim a grant
func (c *HTTPClient) IsWalletReputable(
	ctx context.Context,
	walletID uuid.UUID,
	platform string,
) (bool, error) {
	req, err := c.NewRequest(
		ctx,
		"GET", "v1/reputation/"+walletID.String(),
		nil,
	)
	if err != nil {
		return false, err
	}

	if len(platform) > 0 {
		req.URL.Query().Add("platform", platform)
	}

	var resp IsWalletReputableResponse
	_, err = c.Do(req, &resp)
	if err != nil {
		return false, err
	}

	return resp.IsReputable, nil
}

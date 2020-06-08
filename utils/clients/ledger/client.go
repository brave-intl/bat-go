package ledger

import (
	"context"
	"errors"
	"os"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients"
	"github.com/brave-intl/bat-go/wallet"
	uuid "github.com/satori/go.uuid"
)

// Client abstracts over the underlying client
type Client interface {
	GetWallet(ctx context.Context, id uuid.UUID) (*wallet.Info, error)
}

// HTTPClient wraps http.Client for interacting with the ledger server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (Client, error) {
	serverEnvKey := "LEDGER_SERVER"
	serverURL := os.Getenv("LEDGER_SERVER")
	if len(serverURL) == 0 {
		return nil, errors.New(serverEnvKey + " was empty")
	}
	client, err := clients.New(serverURL, os.Getenv("LEDGER_TOKEN"))
	if err != nil {
		return nil, err
	}
	return NewClientWithPrometheus(&HTTPClient{client}, "ledger_client"), err
}

// WalletAddresses contains the wallet addresses
type WalletAddresses struct {
	ProviderID uuid.UUID `json:"CARD_ID"`
}

// WalletResponse contains information about the ledger wallet
type WalletResponse struct {
	Addresses     WalletAddresses          `json:"addresses"`
	AltCurrency   *altcurrency.AltCurrency `json:"altcurrency"`
	PublicKey     string                   `json:"httpSigningPubKey"`
	PayoutAddress *string                  `json:"anonymousAddress"`
}

// GetWallet retrieves wallet information
func (c *HTTPClient) GetWallet(ctx context.Context, id uuid.UUID) (*wallet.Info, error) {
	req, err := c.client.NewRequest(ctx, "GET", "v2/wallet/"+id.String()+"/info", "", nil)
	if err != nil {
		return nil, err
	}

	var walletResponse WalletResponse
	resp, err := c.client.Do(ctx, req, &walletResponse)

	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, nil
		}
		return nil, err
	}

	info := wallet.Info{
		ID:            id.String(),
		Provider:      "uphold",
		ProviderID:    walletResponse.Addresses.ProviderID.String(),
		AltCurrency:   walletResponse.AltCurrency,
		PublicKey:     walletResponse.PublicKey,
		LastBalance:   nil,
		PayoutAddress: walletResponse.PayoutAddress,
	}

	return &info, err
}

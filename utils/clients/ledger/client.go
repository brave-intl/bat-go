package ledger

import (
	"context"
	"errors"
	"os"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients"
	"github.com/brave-intl/bat-go/utils/wallet"
	uuid "github.com/satori/go.uuid"
)

// Client abstracts over the underlying client
type Client interface {
	GetWallet(ctx context.Context, id uuid.UUID) (*wallet.Info, error)
	GetMemberWallets(ctx context.Context, id uuid.UUID) (*[]wallet.Info, error)
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
	ID               uuid.UUID                `json:"paymentId"`
	Addresses        WalletAddresses          `json:"addresses"`
	AltCurrency      *altcurrency.AltCurrency `json:"altcurrency"`
	PublicKey        string                   `json:"httpSigningPubKey"`
	AnonymousAddress *string                  `json:"anonymousAddress"`
}

// ToInfo converts a wallet response into an info object
func (wr WalletResponse) ToInfo() wallet.Info {
	var anonymousAddress uuid.UUID
	if wr.AnonymousAddress != nil {
		anonymousAddress = uuid.Must(uuid.FromString(*wr.AnonymousAddress))
	}
	return wallet.Info{
		ID:               wr.ID.String(),
		Provider:         "uphold",
		ProviderID:       wr.Addresses.ProviderID.String(),
		AltCurrency:      wr.AltCurrency,
		PublicKey:        wr.PublicKey,
		LastBalance:      nil,
		AnonymousAddress: &anonymousAddress,
	}
}

// GetWallet retrieves wallet information
func (c *HTTPClient) GetWallet(ctx context.Context, id uuid.UUID) (*wallet.Info, error) {
	req, err := c.client.NewRequest(ctx, "GET", "v2/wallet/"+id.String()+"/info", nil)
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
	info := walletResponse.ToInfo()
	return &info, err
}

// GetMemberWallets retrieves wallet information that is linked together through the user id
func (c *HTTPClient) GetMemberWallets(ctx context.Context, id uuid.UUID) (*[]wallet.Info, error) {
	req, err := c.client.NewRequest(ctx, "GET", "v2/wallet/"+id.String()+"/members", nil)
	if err != nil {
		return nil, err
	}

	var walletsResponse []WalletResponse
	resp, err := c.client.Do(ctx, req, &walletsResponse)

	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, nil
		}
		return nil, err
	}
	var wallets []wallet.Info
	for _, wr := range walletsResponse {
		wallets = append(wallets, wr.ToInfo())
	}

	return &wallets, err
}

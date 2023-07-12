package stripe

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/clients"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/google/go-querystring/query"
)

// Client abstracts over the underlying client
type Client interface {
	CreateOnrampSession(
		ctx context.Context,
		integrationMode string,
		walletAddress string,
		sourceCurrency string,
		sourceExchangeAmount string,
		destinationNetwork string,
		destinationCurrency string,
		supportedDestinationNetworks []string,
	) (*OnrampSessionResponse, error)
}

// HTTPClient wraps http.Client for interacting with the Stripe server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
}

// NewWithContext returns a new HTTPClient, retrieving the base URL from the context
func NewWithContext(ctx context.Context) (Client, error) {
	// get the server url from context
	serverURL, err := appctx.GetStringFromContext(ctx, appctx.StripeOnrampServerCTXKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get StripeServer from context: %w", err)
	}

	// get the server secretKey from context
	secretKey, err := appctx.GetStringFromContext(ctx, appctx.StripeOnrampSecretKeyCTXKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get StripeSecretKey from context: %w", err)
	}

	client, err := clients.NewWithHTTPClient(serverURL, secretKey, &http.Client{
		Timeout: time.Second * 30,
	})
	if err != nil {
		return nil, err
	}

	return NewClientWithPrometheus(
		&HTTPClient{
			client: client,
		}, "stripe_onramp_context_client"), nil
}

// onrampSessionParams for fetching prices
type onrampSessionParams struct {
	IntegrationMode              string   `url:"integration_mode"`
	WalletAddress                string   `url:"-"`
	SourceCurrency               string   `url:"transaction_details[source_currency],omitempty"`
	SourceExchangeAmount         string   `url:"transaction_details[source_exchange_amount],omitempty"`
	DestinationNetwork           string   `url:"transaction_details[destination_network],omitempty"`
	DestinationCurrency          string   `url:"transaction_details[destination_currency],omitempty"`
	SupportedDestinationNetworks []string `url:"-"`
}

// GenerateQueryString - implement the QueryStringBody interface
func (p *onrampSessionParams) GenerateQueryString() (url.Values, error) {
	values, err := query.Values(p)
	if err != nil {
		return nil, err
	}
	if p.WalletAddress != "" {
		key := fmt.Sprintf("transaction_details[wallet_addresses][%s]", p.DestinationNetwork)
		values.Add(key, p.WalletAddress)
	}

	if len(p.SupportedDestinationNetworks) > 0 {
		for i, network := range p.SupportedDestinationNetworks {
			key := fmt.Sprintf("transaction_details[supported_destination_networks][%d]", i)
			values.Add(key, network)
		}
	}

	return values, nil
}

// OnrampSessionResponse represents the response received from Stripe
type OnrampSessionResponse struct {
	RedirectURL string `json:"redirect_url"`
}

// CreateOnrampSession creates a new onramp session
func (c *HTTPClient) CreateOnrampSession(
	ctx context.Context,
	integrationMode string,
	walletAddress string,
	sourceCurrency string,
	sourceExchangeAmount string,
	destinationNetwork string,
	destinationCurrency string,
	supportedDestinationNetworks []string,
) (*OnrampSessionResponse, error) {
	url := "/v1/crypto/onramp_sessions"

	params := &onrampSessionParams{
		IntegrationMode:              integrationMode,
		WalletAddress:                walletAddress,
		SourceCurrency:               sourceCurrency,
		SourceExchangeAmount:         sourceExchangeAmount,
		DestinationNetwork:           destinationNetwork,
		DestinationCurrency:          destinationCurrency,
		SupportedDestinationNetworks: supportedDestinationNetworks,
	}

	values, err := params.GenerateQueryString()
	if err != nil {
		return nil, err
	}

	req, err := c.client.NewRequest(ctx, "POST", url, nil, nil)
	if err != nil {
		return nil, err
	}
	// Override request body after req has been created since our client
	// implementation only supports JSON payloads.
	req.Body = ioutil.NopCloser(strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var body OnrampSessionResponse
	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, err
	}

	return &body, nil
}

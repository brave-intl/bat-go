package reputation

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/brave-intl/bat-go/utils/clients"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/google/go-querystring/query"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Client abstracts over the underlying client
type Client interface {
	IsWalletReputable(ctx context.Context, id uuid.UUID, platform string) (bool, error)
	IsWalletAdsReputable(ctx context.Context, id uuid.UUID, platform string) (bool, error)
	IsDrainReputable(ctx context.Context, id, promotionID uuid.UUID, withdrawAmount decimal.Decimal) (bool, string, error)
	IsWalletOnPlatform(ctx context.Context, id uuid.UUID, platform string) (bool, error)
}

// HTTPClient wraps http.Client for interacting with the reputation server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
}

// New returns a new HTTPClient, retrieving the base URL from the
// environment
func New() (Client, error) {
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

	return NewClientWithPrometheus(&HTTPClient{client}, "reputation_client"), nil
}

// IsDrainReputableOpts - the query string options for the is reputable api call
type IsDrainReputableOpts struct {
	WithdrawalAmount string `url:"withdrawal_amount"`
	PromotionID      string `url:"promotion_id"`
}

// GenerateQueryString - implement the QueryStringBody interface
func (iro *IsDrainReputableOpts) GenerateQueryString() (url.Values, error) {
	return query.Values(iro)
}

// IsDrainReputableResponse is what the reputation server
// will send back when we ask if a wallet is reputable
type IsDrainReputableResponse struct {
	Cohort        int    `json:"cohort"`
	Justification string `json:"justification"`
}

// IsDrainReputable makes the request to the reputation server
// and returns whether a paymentId has enough reputation
// to claim a grant
func (c *HTTPClient) IsDrainReputable(
	ctx context.Context,
	paymentID, promotionID uuid.UUID,
	withdrawalAmount decimal.Decimal,
) (bool, string, error) {

	var body = IsDrainReputableOpts{
		WithdrawalAmount: withdrawalAmount.String(),
		PromotionID:      promotionID.String(),
	}

	req, err := c.client.NewRequest(
		ctx,
		"GET",
		"v2/reputation/"+paymentID.String()+"/grants",
		nil,
		&body,
	)
	if err != nil {
		return false, "reputation-service-failure", err
	}

	var resp IsDrainReputableResponse
	_, err = c.client.Do(ctx, req, &resp)
	if err != nil {
		return false, "reputation-service-failures", err
	}

	return resp.Cohort == 1, resp.Justification, nil
}

// IsWalletReputableResponse is what the reputation server
// will send back when we ask if a wallet is reputable
type IsWalletReputableResponse struct {
	IsReputable bool `json:"isReputable"`
}

// IsReputableOpts - the query string options for the is reputable api call
type IsReputableOpts struct {
	Platform string `url:"platform"`
}

// GenerateQueryString - implement the QueryStringBody interface
func (iro *IsReputableOpts) GenerateQueryString() (url.Values, error) {
	return query.Values(iro)
}

// IsWalletAdsReputable makes the request to the reputation server
// and returns whether a paymentId has enough reputation
// to claim a grant
func (c *HTTPClient) IsWalletAdsReputable(
	ctx context.Context,
	paymentID uuid.UUID,
	platform string,
) (bool, error) {

	var body IsReputableOpts
	if platform != "" {
		// pass in query string "platform" into our request
		body = IsReputableOpts{
			Platform: platform,
		}
	}

	req, err := c.client.NewRequest(
		ctx,
		"GET",
		"v1/reputation/"+paymentID.String()+"/ads",
		nil,
		&body,
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

// IsWalletReputable makes the request to the reputation server
// and returns whether a paymentId has enough reputation
// to claim a grant
func (c *HTTPClient) IsWalletReputable(
	ctx context.Context,
	paymentID uuid.UUID,
	platform string,
) (bool, error) {

	var body IsReputableOpts
	if platform != "" {
		// pass in query string "platform" into our request
		body = IsReputableOpts{
			Platform: platform,
		}
	}

	req, err := c.client.NewRequest(
		ctx,
		"GET",
		"v1/reputation/"+paymentID.String(),
		nil,
		&body,
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

// IsWalletOnPlatformResponse - will send back indication if wallet is on said platform
type IsWalletOnPlatformResponse struct {
	IsOnPlatform bool `json:"isOnPlatform"`
}

// IsWalletOnPlatformOpts - the query string options for the is reputable api call
type IsWalletOnPlatformOpts struct {
	PriorTo string `url:"priorTo"`
}

// GenerateQueryString - implement the QueryStringBody interface
func (iwopo *IsWalletOnPlatformOpts) GenerateQueryString() (url.Values, error) {
	return query.Values(iwopo)
}

// IsWalletOnPlatform makes the request to the reputation server
// and returns whether a paymentId is on a given platform
func (c *HTTPClient) IsWalletOnPlatform(
	ctx context.Context,
	paymentID uuid.UUID,
	platform string,
) (bool, error) {

	if platform == "" {
		return false, errors.New("need to specify the platform")
	}

	req, err := c.client.NewRequest(
		ctx,
		"GET",
		fmt.Sprintf("v1/on-platform/%s/%s", platform, paymentID.String()),
		nil,
		&IsWalletOnPlatformOpts{
			PriorTo: ctx.Value(appctx.WalletOnPlatformPriorToCTXKey).(string),
		},
	)
	if err != nil {
		return false, err
	}

	var resp IsWalletOnPlatformResponse
	_, err = c.client.Do(ctx, req, &resp)
	if err != nil {
		return false, err
	}

	return resp.IsOnPlatform, nil
}

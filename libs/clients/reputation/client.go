package reputation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/brave-intl/bat-go/libs/clients"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/google/go-querystring/query"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Client abstracts over the underlying client
type Client interface {
	IsWalletReputable(ctx context.Context, id uuid.UUID, platform string) (bool, error)
	IsWalletAdsReputable(ctx context.Context, id uuid.UUID, platform string) (bool, error)
	IsDrainReputable(ctx context.Context, id, promotionID uuid.UUID, withdrawAmount decimal.Decimal) (bool, []int, error)
	IsLinkingReputable(ctx context.Context, id uuid.UUID, country string) (bool, []int, error)
	IsWalletOnPlatform(ctx context.Context, id uuid.UUID, platform string) (bool, error)
	UpsertReputationSummary(ctx context.Context, paymentID, geoCountry string) error
	UpdateReputationSummary(ctx context.Context, paymentID string, verifiedWallet bool) error
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

// IsReputableResponse is what the reputation server
// will send back when we ask if a wallet is reputable
type IsReputableResponse struct {
	Cohorts []int `json:"cohorts"`
}

var (
	// CohortNil - bad cohort
	CohortNil int
	// CohortOK - ok cohort
	CohortOK = 1
)

// IsLinkingReputableRequestQSB - query string "body" for is linking reputable requests
type IsLinkingReputableRequestQSB struct {
	Country string `url:"country,omitempty"`
}

// GenerateQueryString - implement the QueryStringBody interface
func (ilrrqsb *IsLinkingReputableRequestQSB) GenerateQueryString() (url.Values, error) {
	return query.Values(ilrrqsb)
}

// IsLinkingReputable makes the request to the reputation server
// and returns whether a paymentId has enough reputation
// to claim a grant
func (c *HTTPClient) IsLinkingReputable(
	ctx context.Context,
	paymentID uuid.UUID,
	country string,
) (bool, []int, error) {

	req, err := c.client.NewRequest(
		ctx,
		"GET",
		"v2/reputation/"+paymentID.String()+"/grants",
		nil,
		&IsLinkingReputableRequestQSB{Country: country},
	)
	if err != nil {
		return false, []int{CohortNil}, err
	}

	var resp IsReputableResponse
	_, err = c.client.Do(ctx, req, &resp)
	if err != nil {
		return false, []int{CohortNil}, err
	}

	for _, v := range resp.Cohorts {
		if v == CohortOK {
			return true, resp.Cohorts, nil
		}
	}
	return false, resp.Cohorts, nil
}

// IsDrainReputable makes the request to the reputation server
// and returns whether a paymentId has enough reputation
// to claim a grant
func (c *HTTPClient) IsDrainReputable(
	ctx context.Context,
	paymentID, promotionID uuid.UUID,
	withdrawalAmount decimal.Decimal,
) (bool, []int, error) {

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
		return false, []int{CohortNil}, err
	}

	var resp IsReputableResponse
	_, err = c.client.Do(ctx, req, &resp)
	if err != nil {
		return false, []int{CohortNil}, err
	}

	// okay to be too young for drain reputable
	// must also be ok

	for _, v := range resp.Cohorts {
		if v == CohortOK {
			return true, resp.Cohorts, nil
		}
	}
	return false, resp.Cohorts, nil
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

type reputationSummaryRequest struct {
	GeoCountry string `json:"geoCountry"`
}

// UpsertReputationSummary calls the reputation summary upsert endpoint and creates or updates the reputation
// summary identified by the paymentID with the given geo country.
func (c *HTTPClient) UpsertReputationSummary(ctx context.Context, paymentID, geoCountry string) error {
	b := reputationSummaryRequest{
		GeoCountry: geoCountry,
	}

	req, err := c.client.NewRequest(ctx, http.MethodPut, fmt.Sprintf("v1/reputation-summary/%s", paymentID), b, nil)
	if err != nil {
		return err
	}

	_, err = c.client.Do(ctx, req, nil)
	if err != nil {
		return err
	}

	return nil
}

type reputationSummaryPatchRequest struct {
	VerifiedWallet bool `json:"verifiedWallet"`
}

// UpdateReputationSummary calls the reputation summary update endpoint with the values.
func (c *HTTPClient) UpdateReputationSummary(ctx context.Context, paymentID string, verifiedWallet bool) error {
	r := reputationSummaryPatchRequest{
		VerifiedWallet: verifiedWallet,
	}

	req, err := c.client.NewRequest(ctx, http.MethodPatch, fmt.Sprintf("v1/reputation-summary/%s", paymentID), r, nil)
	if err != nil {
		return err
	}

	_, err = c.client.Do(ctx, req, nil)
	if err != nil {
		return err
	}

	return nil
}

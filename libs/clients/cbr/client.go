package cbr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/brave-intl/bat-go/libs/clients"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
)

// Client abstracts over the underlying client
type Client interface {
	// CreateIssuer creates an issuer.
	CreateIssuer(ctx context.Context, issuer string, maxTokens int) error
	// CreateIssuerV3 creates a version 3 issuer.
	CreateIssuerV3(ctx context.Context, createIssuerV3 IssuerRequest) error
	// GetIssuer returns issuers prior to version 3.
	GetIssuer(ctx context.Context, issuer string) (*IssuerResponse, error)
	// GetIssuerV3 returns issuers based on issuer name. Should be used when retrieving version 3 issuers.
	GetIssuerV3(ctx context.Context, issuer string) (*IssuerResponse, error)
	SignCredentials(ctx context.Context, issuer string, creds []string) (*CredentialsIssueResponse, error)
	RedeemCredential(ctx context.Context, issuer string, preimage string, signature string, payload string) error
	RedeemCredentials(ctx context.Context, credentials []CredentialRedemption, payload string) error
	RedeemCredentialV3(ctx context.Context, issuer string, preimage string, signature string, payload string) error
}

// HTTPClient wraps http.Client for interacting with the cbr server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (Client, error) {
	serverEnvKey := "CHALLENGE_BYPASS_SERVER"
	serverURL := os.Getenv("CHALLENGE_BYPASS_SERVER")
	if len(serverURL) == 0 {
		return nil, errors.New(serverEnvKey + " was empty")
	}
	client, err := clients.New(serverURL, os.Getenv("CHALLENGE_BYPASS_TOKEN"))
	if err != nil {
		return nil, err
	}
	return NewClientWithPrometheus(&HTTPClient{client}, "cbr_client"), err
}

// IssuerCreateRequest is a request to create a new issuer
type IssuerCreateRequest struct {
	Name      string `json:"name"`
	MaxTokens int    `json:"max_tokens"`
}

// IssuerResponse contains detais about a newly created or fetched issuer
type IssuerResponse struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Cohort    int16  `json:"cohort,omitempty"`
}

// CreateIssuer with the provided name and token cap
func (c *HTTPClient) CreateIssuer(ctx context.Context, issuer string, maxTokens int) error {
	req, err := c.client.NewRequest(ctx, "POST", "v1/issuer/", &IssuerCreateRequest{Name: issuer, MaxTokens: maxTokens}, nil)
	if err != nil {
		return err
	}

	_, err = c.client.Do(ctx, req, nil)

	return err
}

// IssuerRequest - create a new issuer request structure
type IssuerRequest struct {
	Name      string     `json:"name"`
	Version   int        `json:"version"`
	Cohort    int16      `json:"cohort"`
	MaxTokens int        `json:"max_tokens"`
	ValidFrom *time.Time `json:"valid_from"` // start of issuance
	ExpiresAt *time.Time `json:"expires_at"`
	Duration  string     `json:"duration"` // valid duration of each sub issuer
	Buffer    int        `json:"buffer"`   // number of sub issuers in the future
	Overlap   int        `json:"overlap"`  // number of days sub issuer should overlap
}

// CreateIssuerV3 creates a version 3 issuer.
func (c *HTTPClient) CreateIssuerV3(ctx context.Context, issuerRequest IssuerRequest) error {
	issuerRequest.Version = 3
	req, err := c.client.NewRequest(ctx, http.MethodPost, "v3/issuer/", issuerRequest, nil)
	if err != nil {
		return fmt.Errorf("error creating create issuer request v3: %w", err)
	}

	_, err = c.client.Do(ctx, req, nil)

	return err
}

// GetIssuer by name
func (c *HTTPClient) GetIssuer(ctx context.Context, issuer string) (*IssuerResponse, error) {
	req, err := c.client.NewRequest(ctx, "GET", "v1/issuer/"+issuer, nil, nil)
	if err != nil {
		return nil, err
	}

	var resp IssuerResponse
	_, err = c.client.Do(ctx, req, &resp)

	return &resp, err
}

// CredentialsIssueRequest is a request to issue more tokens
type CredentialsIssueRequest struct {
	BlindedTokens []string `json:"blinded_tokens"`
}

// CredentialsIssueResponse contains the signed tokens and batch proof
type CredentialsIssueResponse struct {
	BatchProof   string   `json:"batch_proof"`
	SignedTokens []string `json:"signed_tokens"`
}

// GetIssuerV3 returns issuers based on issuer name. Should be used when retrieving version 3 issuers.
func (c *HTTPClient) GetIssuerV3(ctx context.Context, issuer string) (*IssuerResponse, error) {
	req, err := c.client.NewRequest(ctx, http.MethodGet, "v3/issuer/"+issuer, nil, nil)
	if err != nil {
		return nil, err
	}

	var resp IssuerResponse
	_, err = c.client.Do(ctx, req, &resp)

	return &resp, err
}

// SignCredentials using a particular issuer
func (c *HTTPClient) SignCredentials(ctx context.Context, issuer string, creds []string) (*CredentialsIssueResponse, error) {
	req, err := c.client.NewRequest(ctx, "POST", "v1/blindedToken/"+issuer, &CredentialsIssueRequest{BlindedTokens: creds}, nil)
	if err != nil {
		return nil, err
	}

	var resp CredentialsIssueResponse
	_, err = c.client.Do(ctx, req, &resp)

	return &resp, err
}

// CredentialRedeemRequest is a request to redeem a single token toward some payload
type CredentialRedeemRequest struct {
	TokenPreimage string `json:"t"`
	Signature     string `json:"signature"`
	Payload       string `json:"payload"`
}

var (
	// ErrDupRedeem - Error for duplicate redemptions
	ErrDupRedeem = errors.New("cbr duplicate redemption")
	// ErrBadRequest - Error for cbr bad requests
	ErrBadRequest = errors.New("cbr bad request")
)

func handleRedeemError(err error) error {
	var eb *errorutils.ErrorBundle
	if errors.As(err, &eb) {
		if hs, ok := eb.Data().(clients.HTTPState); ok {
			// possible cbr errors:
			// 409 - never retry (already redeemed)
			// 400 - never retry (bad request)
			// 5xx - retry later
			// 429/404 - retry later
			switch hs.Status {
			case http.StatusConflict:
				return errorutils.New(err, ErrDupRedeem.Error(),
					errorutils.Codified{
						ErrCode: "cbr_dup_redeem",
						Retry:   false,
					})
			case http.StatusBadRequest:
				return errorutils.New(err, ErrBadRequest.Error(),
					errorutils.Codified{
						ErrCode: "cbr_bad_request",
						Retry:   false,
					})
			case http.StatusTooManyRequests:
				return errorutils.New(err, "cbr rate limit",
					errorutils.Codified{
						ErrCode: "cbr_rate_limit",
						Retry:   true,
					})
			case http.StatusNotFound:
				return errorutils.New(err, "cbr route not found",
					errorutils.Codified{
						ErrCode: "cbr_path_not_found",
						Retry:   true,
					})
			case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
				return errorutils.New(err, "cbr internal server error",
					errorutils.Codified{
						ErrCode: "cbr_server_err",
						Retry:   true,
					})
			default:
				return errorutils.New(err, "cbr unknown cbr result",
					errorutils.Codified{
						ErrCode: "cbr_unknown",
						Retry:   false,
					})
			}
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return errorutils.New(err, "cbr timeout",
			errorutils.Codified{
				ErrCode: "cbr_timeout",
				Retry:   true,
			})
	}
	return err
}

// RedeemCredential that was issued by the specified issuer
func (c *HTTPClient) RedeemCredential(ctx context.Context, issuer string, preimage string, signature string, payload string) error {
	req, err := c.client.NewRequest(ctx, "POST", "v1/blindedToken/"+issuer+"/redemption/", &CredentialRedeemRequest{TokenPreimage: preimage, Signature: signature, Payload: payload}, nil)
	if err != nil {
		return err
	}

	_, err = c.client.Do(ctx, req, nil)
	return handleRedeemError(err)
}

// CredentialRedemption includes info needed to redeem a single token
type CredentialRedemption struct {
	Issuer        string `json:"issuer"`
	TokenPreimage string `json:"t"`
	Signature     string `json:"signature"`
}

// CredentialsRedeemRequest is a request to redeem one or more tokens toward some payload
type CredentialsRedeemRequest struct {
	Credentials []CredentialRedemption `json:"tokens"`
	Payload     string                 `json:"payload"`
}

// RedeemCredentials that were issued by the specified issuer
func (c *HTTPClient) RedeemCredentials(ctx context.Context, credentials []CredentialRedemption, payload string) error {
	req, err := c.client.NewRequest(ctx, "POST", "v1/blindedToken/bulk/redemption/", &CredentialsRedeemRequest{Credentials: credentials, Payload: payload}, nil)
	if err != nil {
		return err
	}

	_, err = c.client.Do(ctx, req, nil)
	return handleRedeemError(err)
}

// RedeemCredentialV3 redeems a version 3 token that was issued by the specified issuer
func (c *HTTPClient) RedeemCredentialV3(ctx context.Context, issuer string, preimage string, signature string, payload string) error {
	req, err := c.client.NewRequest(ctx, "POST", "v3/blindedToken/"+issuer+"/redemption/",
		&CredentialRedeemRequest{TokenPreimage: preimage, Signature: signature, Payload: payload}, nil)
	if err != nil {
		return err
	}

	_, err = c.client.Do(ctx, req, nil)
	return handleRedeemError(err)
}

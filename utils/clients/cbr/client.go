package cbr

import (
	"context"
	"errors"
	"os"

	"github.com/brave-intl/bat-go/utils/clients"
)

// Client abstracts over the underlying client
type Client interface {
	CreateIssuer(ctx context.Context, issuer string, maxTokens int) error
	GetIssuer(ctx context.Context, issuer string) (*IssuerResponse, error)
	SignCredentials(ctx context.Context, issuer string, creds []string) (*CredentialsIssueResponse, error)
	RedeemCredential(ctx context.Context, issuer string, preimage string, signature string, payload string) error
	RedeemCredentials(ctx context.Context, credentials []CredentialRedemption, payload string) error
}

// HTTPClient wraps http.Client for interacting with the ledger server
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
}

// CreateIssuer with the provided name and token cap
func (c *HTTPClient) CreateIssuer(ctx context.Context, issuer string, maxTokens int) error {
	req, err := c.client.NewRequest(ctx, "POST", "v1/issuer/", "", &IssuerCreateRequest{Name: issuer, MaxTokens: maxTokens})
	if err != nil {
		return err
	}

	_, err = c.client.Do(ctx, req, nil)

	return err
}

// GetIssuer by name
func (c *HTTPClient) GetIssuer(ctx context.Context, issuer string) (*IssuerResponse, error) {
	req, err := c.client.NewRequest(ctx, "GET", "v1/issuer/"+issuer, "", nil)
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

// SignCredentials using a particular issuer
func (c *HTTPClient) SignCredentials(ctx context.Context, issuer string, creds []string) (*CredentialsIssueResponse, error) {
	req, err := c.client.NewRequest(ctx, "POST", "v1/blindedToken/"+issuer, "", &CredentialsIssueRequest{BlindedTokens: creds})
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

// RedeemCredential that was issued by the specified issuer
func (c *HTTPClient) RedeemCredential(ctx context.Context, issuer string, preimage string, signature string, payload string) error {
	req, err := c.client.NewRequest(ctx, "POST", "v1/blindedToken/"+issuer+"/redemption/", "", &CredentialRedeemRequest{TokenPreimage: preimage, Signature: signature, Payload: payload})
	if err != nil {
		return err
	}

	_, err = c.client.Do(ctx, req, nil)

	return err
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
	req, err := c.client.NewRequest(ctx, "POST", "v1/blindedToken/bulk/redemption/", "", &CredentialsRedeemRequest{Credentials: credentials, Payload: payload})
	if err != nil {
		return err
	}

	_, err = c.client.Do(ctx, req, nil)

	return err
}

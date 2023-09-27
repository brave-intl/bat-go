package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/shopspring/decimal"
)

// ErrHeaderNotFound is the error returned when an expected response header cannot be found.
var ErrHeaderNotFound = errors.New("payment: header not found")

type (
	Details struct {
		To        string          `json:"to"`
		From      string          `json:"from"`
		Amount    decimal.Decimal `json:"amount"`
		Custodian string          `json:"custodian"`
		PayoutID  string          `json:"payoutId"`
		Currency  string          `json:"currency"`
	}

	// SerializedDetails represent payment details which have been serialized.
	SerializedDetails = []byte

	// AttestedDetails represents payment details that have been attested.
	AttestedDetails struct {
		Details
		DocumentID          string `json:"documentId"`
		AttestationDocument string `json:"attestationDocument"` // base64 encoded
	}
)

// AuthorizationHeader headers used to authorize a submit request.
type AuthorizationHeader = map[string][]string

// MarshalBinary implements encoding.BinaryMarshaler required for go-redis.
func (a AttestedDetails) MarshalBinary() (data []byte, err error) {
	b, err := json.Marshal(a)
	if err != nil {
		return nil, fmt.Errorf("event message: error marshalling binary: %w", err)
	}
	return b, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler required for go-redis.
func (a *AttestedDetails) UnmarshalBinary(data []byte) error {
	err := json.Unmarshal(data, a)
	if err != nil {
		return fmt.Errorf("event message: error unmarshalling binary: %w", err)
	}
	return nil
}

type Client struct {
	httpClient *clients.SimpleHTTPClient
}

// NewClient returns a new instance of payment client wrapped with Prometheus.
func NewClient(baseURL string) (*Client, error) {
	simpleHTTPClient, err := clients.New(baseURL, "")
	if err != nil {
		return nil, err
	}
	return &Client{
		httpClient: simpleHTTPClient,
	}, nil
}

// Prepare payment details to be processed.
func (c *Client) Prepare(ctx context.Context, sd SerializedDetails) (AttestedDetails, error) {
	resource := c.httpClient.BaseURL.ResolveReference(&url.URL{
		Path: "/v1/payments/prepare",
	})

	req, err := http.NewRequest(http.MethodPost, resource.String(), bytes.NewBuffer(sd))
	if err != nil {
		return AttestedDetails{}, fmt.Errorf("error creating new prepare request: %w", err)
	}

	var aTxn AttestedDetails
	resp, err := c.httpClient.Do(ctx, req, &aTxn)
	if err != nil {
		return AttestedDetails{}, fmt.Errorf("error calling prepare endpoint: %w", err)
	}

	const (
		xAttestHeader = "X-Nitro-Attestation"
	)

	at := resp.Header.Get(xAttestHeader)
	if at == "" {
		return AttestedDetails{}, fmt.Errorf("error retrieving attestation header: %w", err)
	}
	aTxn.AttestationDocument = at

	return aTxn, nil
}

// Submit is the response returned by the submit endpoint.
type Submit struct {
	RetryAfter time.Duration
}

// IsComplete returns true when a transaction has been both successfully submitted and processed;
// it returns false otherwise.
func (s *Submit) IsComplete() bool {
	return s.RetryAfter == 0
}

// Submit calls the payment service submit endpoint with the authorization headers and the transaction to be submitted.
// The parameters should be a serialized attested transaction and its related authorization headers.
// If the Submit request returns a http.StatusAccepted and a XRetryAfter header then the transaction has been
// successfully submitted but processing has not yet been complete. When a transaction has been successful submitted
// and processing is complete both return values will be nil. Submit is idempotent and can be called many times with
// the intention of both submitting and checking the transaction status.
func (c *Client) Submit(ctx context.Context, authHeader AuthorizationHeader, sd SerializedDetails) (Submit, error) {
	resource := c.httpClient.BaseURL.ResolveReference(&url.URL{
		Path: "/v1/payments/submit",
	})

	req, err := http.NewRequest(http.MethodPost, resource.String(), bytes.NewBuffer(sd))
	if err != nil {
		return Submit{}, fmt.Errorf("error creating new submit request: %w", err)
	}
	req.Header = authHeader

	resp, err := c.httpClient.Do(ctx, req, nil)
	if err != nil {
		return Submit{}, fmt.Errorf("error calling submit request: %w", err)
	}

	if resp.StatusCode == http.StatusAccepted {
		ra, err := parseXSubmitRetryAfter(resp)
		if err != nil {
			return Submit{}, fmt.Errorf("error parsing header: %w", err)
		}
		return Submit{RetryAfter: time.Duration(ra)}, nil
	}

	return Submit{}, nil
}

// parseXSubmitRetryAfter extracts the xSubmitRetryAfterHeader value from a http.Response and returns
// The retry after value must a positive number to parse successfully.
func parseXSubmitRetryAfter(resp *http.Response) (uint64, error) {
	const (
		xSubmitRetryAfterHeader = "X-Submit-Retry-After"
	)

	ra := resp.Header.Get(xSubmitRetryAfterHeader)
	if ra == "" {
		return 0, fmt.Errorf("error retrieving %s: %w", xSubmitRetryAfterHeader, ErrHeaderNotFound)
	}

	u, err := strconv.ParseUint(ra, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing unsigned int: %w", err)
	}

	return u, nil
}

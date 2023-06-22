package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/ptr"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// XSubmitRetryAfter is the response header we expect from the payment service to denote the minimum amount of time
// we should wait before retying calling submit. This specific to the submit endpoint and distinct from more
// generalised the retry after that can be returned by some endpoints.
const XSubmitRetryAfter = "x-submit-retry-after"

var defaultXRetryAfter = ptr.FromDuration(time.Duration(0))

type (
	// Transaction represent a transaction that has been serialized.
	Transaction = []byte

	// AttestedTransaction is a transaction that has been attested.
	AttestedTransaction struct {
		IdempotencyKey      uuid.UUID       `json:"idempotencyKey"`
		Custodian           string          `json:"custodian"`
		To                  uuid.UUID       `json:"to"`
		Amount              decimal.Decimal `json:"amount"`
		DocumentID          string          `json:"documentId"`
		Version             string          `json:"version"`
		State               string          `json:"state"`
		AttestationDocument string          `json:"attestationDocument"` // base64 encoded
		DryRun              *string         `json:"dryRun" ion:"-"`
	}

	// AuthorizationHeader headers used to authorize a submit request.
	AuthorizationHeader = map[string][]string
)

// MarshalBinary implements encoding.BinaryMarshaler required for go-redis.
func (m AttestedTransaction) MarshalBinary() (data []byte, err error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("event message: error marshalling binary: %w", err)
	}
	return b, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler required for go-redis.
func (m *AttestedTransaction) UnmarshalBinary(data []byte) error {
	err := json.Unmarshal(data, m)
	if err != nil {
		return fmt.Errorf("event message: error unmarshalling binary: %w", err)
	}
	return nil
}

type Client interface {
	Prepare(ctx context.Context, transaction Transaction) (AttestedTransaction, error)
	Submit(ctx context.Context, authorizationHeader AuthorizationHeader, transaction Transaction) (*time.Duration, error)
}

type client struct {
	httpClient *clients.SimpleHTTPClient
}

// New returns a new instance of payment client wrapped with Prometheus.
func New(baseURL string) Client {
	simpleHTTPClient, _ := clients.New(baseURL, "")
	return NewClientWithPrometheus(&client{httpClient: simpleHTTPClient},
		"payment_client")
}

// Prepare transactions to be processed. Prepare returns a zero value AttestedTransaction when an error occurs.
func (pc *client) Prepare(ctx context.Context, transaction Transaction) (AttestedTransaction, error) {
	resource := pc.httpClient.BaseURL.ResolveReference(&url.URL{
		Path: "/v1/payments/prepare",
	})

	request, err := http.NewRequest(http.MethodPost, resource.String(), bytes.NewBuffer(transaction))
	if err != nil {
		return AttestedTransaction{}, fmt.Errorf("error creating new prepare request: %w", err)
	}

	var aTxn AttestedTransaction
	_, err = pc.httpClient.Do(ctx, request, &aTxn)
	if err != nil {
		return AttestedTransaction{}, fmt.Errorf("error calling prepare request: %w", err)
	}

	return aTxn, nil
}

// Submit calls the payment service submit endpoint with the authorization headers and the transaction to be submitted.
// The parameters should be a serialized attested transaction and its related authorization headers.
// If the Submit request returns a http.StatusAccepted and a XSubmitRetryAfter header value has been
// provided then this value will be returned. For a successful call both return values will be nil.
func (pc *client) Submit(ctx context.Context, authorizationHeader AuthorizationHeader, transaction Transaction) (*time.Duration, error) {
	resource := pc.httpClient.BaseURL.ResolveReference(&url.URL{
		Path: "/v1/payments/submit",
	})

	request, err := http.NewRequest(http.MethodPost, resource.String(), bytes.NewBuffer(transaction))
	if err != nil {
		return nil, fmt.Errorf("error creating new submit request: %w", err)
	}
	request.Header = authorizationHeader

	//TODO sync up once payment has implemented to error codes to make sure we are retrying the correct responses.

	response, err := pc.httpClient.Do(ctx, request, nil)
	if err != nil {
		return nil, fmt.Errorf("error calling submit request: %w", err)
	}

	// If the http status is a 202 extract the x-submit-retry-after value.
	// In the eventuality the status is a http.StatusAccepted but no value has been set
	// return the default.
	if response.StatusCode == http.StatusAccepted {
		h, ok := response.Header[XSubmitRetryAfter]
		if !ok {
			return defaultXRetryAfter, nil
		}

		if len(h) != 1 {
			return nil, fmt.Errorf("error invalid header length: %s", h)
		}

		u, err := strconv.ParseUint(h[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing %s: %w", XSubmitRetryAfter, err)
		}

		return ptr.FromDuration(time.Duration(u)), nil
	}

	return nil, nil
}

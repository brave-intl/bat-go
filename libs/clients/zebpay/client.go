package zebpay

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/requestutils"
	jose "github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shopspring/decimal"
)

var (
	balanceGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "zebpay_account_balance",
		Help: "A gauge of the current account balance in zebpay",
	})
)

func init() {
	prometheus.MustRegister(balanceGauge)
}

// BulkTransfer is the structure of a bulk transfer for zebpay
type BulkTransferRequest []*Transfer

/*
from the api specification
[
     {
          "transaction_id": "c6911095-ba83-4aa1-b0fb-15934568a943",
          "destination": 512,
          "amount": "163",
          "from": "c6911095-ba83-4aa1-b0fb-15934568a65a"
     }
]
*/

// NewBulkTransferRequest returns a bulk transfer out of a list of transfers
func NewBulkTransferRequest(transfers ...*Transfer) BulkTransferRequest {
	return BulkTransferRequest(transfers)
}

// Transfer is the structure of a transfer for zebpay
type Transfer struct {
	ID          uuid.UUID       `json:"transaction_id"`
	Destination int64           `json:"destination"`
	Amount      decimal.Decimal `json:"amount"`
	From        uuid.UUID       `json:"from"`
}

// NewTransfer creates and returns a new transfer from parameters
func NewTransfer(id, from uuid.UUID, amount decimal.Decimal, destination int64) *Transfer {
	return &Transfer{
		ID: id, From: from, Amount: amount, Destination: destination,
	}
}

// BulkTransferResponse is the response structure of the bulk transfer api call
type BulkTransferResponse struct {
	Data string `json:"data"`
}

/*
{
    "data": "ALL_SENT_TRANSACTIONS_ACKNOWLEDGED"
}
*/

// BulkCheckTransferResponse is the response structure from a bulk status check
type BulkCheckTransferResponse []*CheckTransferResponse

// CheckTransferResponse is the structure of a transfer response
type CheckTransferResponse struct {
	ID        uuid.UUID `json:"transaction_id"`
	Error     string    `json:"error"`
	ErrorCode string    `json:"error_code"`
	Code      int64     `json:"code"`
	Status    string    `json:"status"`
	Details   Transfer  `json:"details"`
}

/*
{
   "transaction_id": "61e98be4-ea84-43d6-9b08-ef5cd95a55e2",
   "code": 2,
   "status": "Success",
   "details": {
       "amount": 13.736461457342187,
       "destination": 60
   }
}
*/

var (
	CheckTransferCodeToStatus = map[int]string{
		TransferPendingCode: TransferPendingStatus,
		TransferSuccessCode: TransferSuccessStatus,
		TransferFailedCode:  TransferFailedStatus,
	}
)

const (
	// TransferPendingCode is the status code for pending status
	TransferPendingCode = 1
	// TransferSuccessCode is the status code for successful transfer
	TransferSuccessCode = 2
	// TransferFailedCode is the status code for failed transfer
	TransferFailedCode = 3
	// TransferNotFoundCode is the status code for transfer not found
	TransferNotFoundCode = 404

	// TransferPendingStatus is the status code for pending status
	TransferPendingStatus = "Pending"
	// TransferSuccessStatus is the status code for successful transfer
	TransferSuccessStatus = "Success"
	// TransferFailedStatus is the status code for failed transfer
	TransferFailedStatus = "Failed"
	// TransferNotFoundStatus is the status code for transfer not found
	TransferNotFoundStatus = "NotFound"
)

// ClientOpts are the common configuration options for the Zebpay Client
type ClientOpts struct {
	APIKey     string
	SigningKey crypto.PrivateKey
}

// Client abstracts over the underlying client
type Client interface {
	// BulkTransfer posts a bulk transfer request to zebpay
	BulkTransfer(ctx context.Context, opts *ClientOpts, req BulkTransferRequest) (*BulkTransferResponse, error)
	// BulkCheckTransfer checks the status of a transaction
	BulkCheckTransfer(ctx context.Context, opts *ClientOpts, ids ...uuid.UUID) (BulkCheckTransferResponse, error)
	// CheckTransfer checks the status of a transaction
	CheckTransfer(ctx context.Context, opts *ClientOpts, id uuid.UUID) (*CheckTransferResponse, error)
}

// HTTPClient wraps http.Client for interacting with the cbr server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (Client, error) {
	serverEnvKey := "ZEBPAY_SERVER"
	serverURL := os.Getenv(serverEnvKey)
	if len(serverURL) == 0 {
		return nil, errors.New(serverEnvKey + " was empty")
	}
	proxy := os.Getenv("HTTP_PROXY")
	client, err := clients.NewWithProxy("zebpay", serverURL, "", proxy) // authentication bearer token is set per api call
	if err != nil {
		return nil, err
	}
	return NewClientWithPrometheus(&HTTPClient{client}, "zebpay_client"), err
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func NewWithHTTPClient(httpClient http.Client) (Client, error) {
	serverEnvKey := "ZEBPAY_SERVER"
	serverURL := os.Getenv(serverEnvKey)
	if len(serverURL) == 0 {
		return nil, errors.New(serverEnvKey + " was empty")
	}
	client, err := clients.NewWithHTTPClient(serverURL, "", &httpClient) // authentication bearer token is set per api call
	if err != nil {
		return nil, err
	}
	return NewClientWithPrometheus(&HTTPClient{client}, "zebpay_client"), err
}

var errBadClientOpts = errors.New("client is misconfigured with no client options")

type claims struct {
	*jwt.Claims           // use iat, exp, sub - same as x-api-key
	URI         string    `json:"uri,omitempty"`      // the endpoint called
	Nonce       uuid.UUID `json:"nonce,omitempty"`    // one time use uuid
	BodyHash    string    `json:"bodyHash,omitempty"` //Hex-encoded SHA-256 hash of the raw HTTP request body.
}

// affixAccessToken will take a request and generate a zebpay access token and affix it to the
// headers of the request
func affixAccessToken(req *http.Request, opts *ClientOpts) error {
	if opts == nil {
		return errBadClientOpts
	}
	var body []byte
	var err error
	if req.Body != nil {
		body, err = requestutils.Read(context.Background(), req.Body)
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}
		req.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	}

	// sha body bytes
	h := sha256.New()
	h.Write(body)
	shasum := fmt.Sprintf("%x", h.Sum(nil))

	// add the api key to the request
	req.Header.Set("X-API-KEY", opts.APIKey)

	// setup the signer options
	key := jose.SigningKey{Algorithm: jose.RS256, Key: opts.SigningKey}
	var signerOpts = jose.SignerOptions{}
	signerOpts.WithType("JWT")

	// create a new signer with the signing key
	rsaSigner, err := jose.NewSigner(key, &signerOpts)
	if err != nil {
		return fmt.Errorf("failed to create jwt signer: %w", err)
	}

	// attach the rsa signer to the builder
	builder := jwt.Signed(rsaSigner)

	// build out the claims of the token
	builder = builder.Claims(claims{
		Claims: &jwt.Claims{
			Subject:  opts.APIKey,
			IssuedAt: jwt.NewNumericDate(time.Now()),
			Expiry:   jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		URI:      req.URL.Path,
		Nonce:    uuid.New(),
		BodyHash: shasum,
	})
	accessToken, err := builder.CompactSerialize()
	if err != nil {
		return fmt.Errorf("failed to create jwt: %w", err)
	}

	// add the authentication bearer token we created
	req.Header.Set("Authorization", "Bearer "+accessToken)

	return nil
}

// CheckTransfer uploads the bulk payout for gemini
func (c *HTTPClient) CheckTransfer(ctx context.Context, opts *ClientOpts, id uuid.UUID) (*CheckTransferResponse, error) {
	resp := &CheckTransferResponse{ID: id}
	if opts == nil {
		resp.Error = errBadClientOpts.Error()
		return resp, errBadClientOpts
	}

	// /api/checktransferstatus/<UUID>/status

	urlPath := fmt.Sprintf("/api/checktransferstatus/%s/status", id.String())
	req, err := c.client.NewRequest(ctx, "GET", urlPath, nil, nil)
	if err != nil {
		resp.Error = err.Error()
		return resp, err
	}

	// populate the access token
	affixAccessToken(req, opts)

	httpResponse, err := c.client.Do(ctx, req, resp)
	if err != nil {
		resp.Error = err.Error()
		resp.Code = int64(httpResponse.StatusCode)
	}

	return resp, err
}

// BulkCheckTransfer uploads the bulk payout for gemini
func (c *HTTPClient) BulkCheckTransfer(ctx context.Context, opts *ClientOpts, ids ...uuid.UUID) (BulkCheckTransferResponse, error) {
	if opts == nil {
		return nil, errBadClientOpts
	}

	// /api/bulktransfercheckstatus
	req, err := c.client.NewRequest(ctx, "GET", "/api/bulktransfercheckstatus", ids, nil)
	if err != nil {
		return nil, err
	}

	// populate the access token
	affixAccessToken(req, opts)

	var resp = BulkCheckTransferResponse{}
	_, err = c.client.Do(ctx, req, &resp)

	return resp, err
}

// BulkTransfer performs a bulk transfer for payouts
func (c *HTTPClient) BulkTransfer(ctx context.Context, opts *ClientOpts, transfers BulkTransferRequest) (*BulkTransferResponse, error) {
	if opts == nil {
		return nil, errBadClientOpts
	}

	// check if any transfer exceeds our limit
	for _, v := range transfers {
		if v.Amount.GreaterThan(clients.TransferLimit) {
			return nil, clients.ErrTransferExceedsLimit
		}
	}

	// /api/bulktransfercheckstatus
	req, err := c.client.NewRequest(ctx, "POST", "/api/bulktransfer", transfers, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to construct new request: %w", err)
	}

	// populate the access token
	affixAccessToken(req, opts)

	var resp = new(BulkTransferResponse)
	_, err = c.client.Do(ctx, req, resp)
	if err != nil {
		return nil, fmt.Errorf("failed to do transfer request: %w", err)
	}

	return resp, err
}

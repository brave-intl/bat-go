package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/brave-intl/bat-go/utils/clients"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Client defines the methods used to call the payment service
type Client interface {
	Prepare(ctx context.Context, transactions []Transaction) (*[]Transaction, error)
	Submit(ctx context.Context, transactions []Transaction) error
	Status(ctx context.Context, documentID string) (*TransactionStatus, error)
}

type (
	// Transaction represents a single transaction in the system
	Transaction struct {
		IdempotencyKey uuid.UUID       `json:"idempotencyKey"`
		Custodian      *string         `json:"custodian,omitempty"`
		Amount         decimal.Decimal `json:"amount"`
		To             uuid.UUID       `json:"to"`
		From           uuid.UUID       `json:"from"`
		DocumentID     *string         `json:"documentId,omitempty"`
	}

	// TransactionStatus contains information about the state of a transaction
	TransactionStatus struct {
		// Transaction contains the details of the transaction
		Transaction Transaction `json:"transaction"`
		// CustodianSubmissionResponse raw response when transaction was submitted
		CustodianSubmissionResponse *string `json:"submissionResponse,omitempty"`
		// CustodianStatusResponse raw response for check status
		CustodianStatusResponse *string `json:"statusResponse,omitempty"`
	}
)

// Error payments service error response
type Error struct {
	// Http status code
	Code int `json:"code"`
	// Message containing error description
	Message string `json:"message"`
	// Underlying custodian error
	Data interface{} `json:"data"`
}

type paymentClient struct {
	httpClient            *clients.SimpleHTTPClient
	parameterizedSignator httpsignature.ParameterizedSignator
}

// New returns a new instance of payment client
func New(baseURL string, parameterizedSignator httpsignature.ParameterizedSignator) Client {
	simpleHTTPClient, _ := clients.New(baseURL, "")
	return NewClientWithPrometheus(&paymentClient{httpClient: simpleHTTPClient,
		parameterizedSignator: parameterizedSignator},
		"payment_client")
}

// Prepare transactions to be processed
func (pc *paymentClient) Prepare(ctx context.Context, transactions []Transaction) (*[]Transaction, error) {
	request, err := pc.httpClient.NewRequest(ctx, http.MethodPost, "/v1/payments/prepare", transactions, nil)
	if err != nil {
		return nil, err
	}

	var transactionResponse []Transaction
	_, err = pc.httpClient.Do(ctx, request, &transactionResponse)
	if err != nil {
		return nil, err
	}

	return &transactionResponse, nil
}

// Submit transactions to be processed
func (pc *paymentClient) Submit(ctx context.Context, transactions []Transaction) error {
	request, err := pc.httpClient.NewRequest(ctx, http.MethodPost, "/v1/payments/submit", transactions, nil)
	if err != nil {
		return err
	}

	err = pc.parameterizedSignator.Sign(pc.parameterizedSignator.Signator,
		pc.parameterizedSignator.Opts, request)
	if err != nil {
		return fmt.Errorf("error signing http request: %w", err)
	}

	_, err = pc.httpClient.Do(ctx, request, nil)
	if err != nil {
		return err
	}

	return nil
}

// Status checks the status of a transaction identified by documentID
func (pc *paymentClient) Status(ctx context.Context, documentID string) (*TransactionStatus, error) {
	request, err := pc.httpClient.NewRequest(ctx, http.MethodGet,
		fmt.Sprintf("/v1/payments/%s/status", documentID), nil, nil)
	if err != nil {
		return nil, err
	}

	var transactionStatus TransactionStatus
	_, err = pc.httpClient.Do(ctx, request, &transactionStatus)
	if err != nil {
		return nil, err
	}

	return &transactionStatus, nil
}

// UnwrapPaymentError this is a helper func to retrieve the payment.Error from an error bundle
func UnwrapPaymentError(err error) (*Error, error) {
	errorData, err := clients.UnwrapErrorData(err)
	if err != nil {
		return nil, fmt.Errorf("error unwrapping error data: %w", err)
	}
	if s, ok := errorData.Body.(string); ok {
		var paymentError Error
		err = json.Unmarshal([]byte(s), &paymentError)
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling error data: %w", err)
		}
		return &paymentError, nil
	}
	return nil, fmt.Errorf("error retrieving error data body: %w", err)
}

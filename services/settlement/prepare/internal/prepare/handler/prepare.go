package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/libs/backoff/retrypolicy"
	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/clients/payment"
	"github.com/brave-intl/bat-go/services/settlement/event"
	"github.com/brave-intl/bat-go/services/settlement/payout"
)

var (
	retryPolicy        = retrypolicy.DefaultRetry
	nonRetriableErrors = []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden}
)

type PreparedTransactionAPI interface {
	AddPreparedTransaction(ctx context.Context, payoutID string, attestedTransaction payment.AttestedTransaction) error
}

// PaymentClient defines the methods used to call the payment service.
type PaymentClient interface {
	Prepare(ctx context.Context, transaction payment.Transaction) (payment.AttestedTransaction, error)
}

type prepare struct {
	preparedTransactionAPI PreparedTransactionAPI
	paymentClient          PaymentClient
	config                 payout.Config
	retry                  backoff.RetryFunc
}

func NewHandler(preparedTransactionAPI PreparedTransactionAPI, paymentClient PaymentClient, config payout.Config, retry backoff.RetryFunc) event.Handler {
	return &prepare{
		preparedTransactionAPI: preparedTransactionAPI,
		paymentClient:          paymentClient,
		config:                 config,
		retry:                  retry,
	}
}

func (p *prepare) Handle(ctx context.Context, message event.Message) error {
	transaction := payment.Transaction(message.Body)

	prepareOperation := func() (interface{}, error) {
		return p.paymentClient.Prepare(ctx, transaction)
	}

	response, err := p.retry(ctx, prepareOperation, retryPolicy, canRetry(nonRetriableErrors))
	if err != nil {
		return fmt.Errorf("prepare handler: error calling payment service: %w", err)
	}

	attestedTransaction, ok := response.(payment.AttestedTransaction)
	if !ok {
		return fmt.Errorf("error type conversion: %w", err)
	}

	err = p.preparedTransactionAPI.AddPreparedTransaction(ctx, p.config.PayoutID, attestedTransaction)
	if err != nil {
		return fmt.Errorf("prepare handler: error calling zaddnx: %w", err)
	}

	return nil
}

func canRetry(nonRetriableErrors []int) func(error) bool {
	return func(err error) bool {
		if httpState, err := clients.UnwrapHTTPState(err); err == nil {
			for _, httpStatusCode := range nonRetriableErrors {
				if httpState.Status == httpStatusCode {
					return false
				}
			}
		}
		return true
	}
}

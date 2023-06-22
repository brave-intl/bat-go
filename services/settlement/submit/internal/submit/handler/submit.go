package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/libs/backoff/retrypolicy"
	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/clients/payment"
	"github.com/brave-intl/bat-go/services/settlement/event"
)

var (
	retryPolicy = retrypolicy.DefaultRetry

	// nonRetriableErrors represents the errors which should not be retried.
	nonRetriableErrors = []int{http.StatusBadRequest, http.StatusUnauthorized,
		http.StatusForbidden}
)

// PaymentClient defines the methods used to call the payment service.
type PaymentClient interface {
	Submit(ctx context.Context, authorizationHeader payment.AuthorizationHeader, transaction payment.Transaction) (*time.Duration, error)
}

type submit struct {
	redis   *event.RedisClient
	payment PaymentClient
	retry   backoff.RetryFunc
}

func NewHandler(redis *event.RedisClient, payment PaymentClient, retry backoff.RetryFunc) event.Handler {
	return &submit{
		redis:   redis,
		payment: payment,
		retry:   retry,
	}
}

func (s *submit) Handle(ctx context.Context, message event.Message) error {
	ah := make(payment.AuthorizationHeader)
	for k, v := range message.Headers {
		ah[k] = []string{v}
	}
	transaction := payment.Transaction(message.Body)

	submitOperation := func() (interface{}, error) {
		return s.payment.Submit(ctx, ah, transaction)
	}

	r, err := s.retry(ctx, submitOperation, retryPolicy, canRetry(nonRetriableErrors))
	if err != nil {
		return fmt.Errorf("submit handler: error calling payment service: %w", err)
	}

	seconds, ok := r.(*time.Duration)
	if !ok {
		//TODO fix error
		return fmt.Errorf("submit handler: error type conversion retry after: %w", err)
	}

	if seconds != nil {
		return &event.RetryError{
			RetryAfter: *seconds,
		}
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

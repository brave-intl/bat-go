package handler

import (
	"context"
	"errors"
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

type PaymentClient interface {
	Submit(ctx context.Context, authorizationHeader payment.AuthorizationHeader, transaction payment.Transaction) (*time.Duration, error)
}

// Submit implements a submit event handler.
type Submit struct {
	redis   *event.RedisClient
	payment PaymentClient
	retry   backoff.RetryFunc
}

// NewHandler returns a new instance of Submit.
func NewHandler(redis *event.RedisClient, payment PaymentClient, retry backoff.RetryFunc) *Submit {
	return &Submit{
		redis:   redis,
		payment: payment,
		retry:   retry,
	}
}

// Handle submits attested transaction to the payment service submit endpoint. When a transaction can be
// resubmitted or retried an event.RetryError is returned otherwise the underlying client error is returned.
func (s *Submit) Handle(ctx context.Context, message event.Message) error {
	ah := make(payment.AuthorizationHeader)
	for k, v := range message.Headers {
		ah[k] = []string{v}
	}
	t := payment.Transaction(message.Body)

	submitOperation := func() (interface{}, error) {
		return s.payment.Submit(ctx, ah, t)
	}

	r, err := s.retry(ctx, submitOperation, retryPolicy, canRetry(nonRetriableErrors))
	if err != nil {
		return fmt.Errorf("submit handler: error calling payment service: %w", err)
	}

	d, ok := r.(*time.Duration)
	if !ok {
		return errors.New("submit handler: error asserting type assertion")
	}

	// If the duration is not nil then we need to retry.
	if d != nil {
		return &event.RetryError{
			RetryAfter: *d,
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

package errored

import (
	"context"
	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/utils/backoff"
	"github.com/brave-intl/bat-go/utils/backoff/retrypolicy"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	"net/http"
)

var (
	retryPolicy        = retrypolicy.DefaultRetry
	nonRetriableErrors = []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden}
)

type errored struct {
	redis   *event.Client
	payment payment.Client
	retry   backoff.RetryFunc
}

func newHandler(redis *event.Client, payment payment.Client, retry backoff.RetryFunc) *errored {
	return &errored{
		redis:   redis,
		payment: payment,
		retry:   retry,
	}
}

func (e *errored) Handle(ctx context.Context, messages []event.Message) error {
	// read in messages

	// get the custodian

	// determine if runnable

	// rewind

	// dlq

	return nil
}

func canRetry(nonRetriableErrors []int) func(error) bool {
	return func(err error) bool {
		if paymentError, err := payment.UnwrapPaymentError(err); err == nil {
			for _, httpStatusCode := range nonRetriableErrors {
				if paymentError.Code == httpStatusCode {
					return false
				}
			}
		}
		return true
	}
}

package notify

import (
	"context"
	"net/http"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/utils/backoff"
	"github.com/brave-intl/bat-go/utils/backoff/retrypolicy"
	"github.com/brave-intl/bat-go/utils/clients/payment"
)

var (
	retryPolicy        = retrypolicy.DefaultRetry
	nonRetriableErrors = []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden}
)

type prepare struct {
	redis   *event.Client
	payment payment.Client
	retry   backoff.RetryFunc
}

func newHandler(redis *event.Client, payment payment.Client, retry backoff.RetryFunc) *prepare {
	return &prepare{
		redis:   redis,
		payment: payment,
		retry:   retry,
	}
}

func (p *prepare) Handle(ctx context.Context, messages []event.Message) error {
	// send to slack
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

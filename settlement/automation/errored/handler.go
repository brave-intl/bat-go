package errored

import (
	"context"
	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/utils/backoff"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	"github.com/brave-intl/bat-go/utils/logging"
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
	logging.FromContext(ctx).Info().
		Interface("messages", messages)
	return nil
}

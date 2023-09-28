package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer"
	"github.com/brave-intl/bat-go/services/settlement/internal/payment"
)

var (
	retryRespCodes = map[int]struct{}{
		http.StatusRequestTimeout:      {},
		http.StatusTooEarly:            {},
		http.StatusTooManyRequests:     {},
		http.StatusInternalServerError: {},
		http.StatusBadGateway:          {},
		http.StatusServiceUnavailable:  {},
		http.StatusGatewayTimeout:      {},
		http.StatusForbidden:           {},
	}

	ErrSubmitNotComplete = errors.New("handler: submit not complete")
)

type PaymentClient interface {
	Submit(ctx context.Context, authorizationHeader payment.AuthorizationHeader, details payment.SerializedDetails) (payment.Submit, error)
}

// Handler implements a submit stream handler.
type Handler struct {
	payment PaymentClient
}

// New returns a new instance of Handler.
func New(payment PaymentClient) *Handler {
	return &Handler{
		payment: payment,
	}
}

// Handle submits attested transaction to the payment service submit endpoint. When a transaction can be
// resubmitted or retried a consumer.RetryError is returned otherwise the underlying client error is returned.
// Transactions will be retired when we receive a status code as defined in the retriable errors
// e.g. http.StatusTooManyRequests and also when a transaction has been successfully submitted but not fully processed.
func (h *Handler) Handle(ctx context.Context, message consumer.Message) error {
	const (
		minRetryAfter = 1
	)

	ah := make(payment.AuthorizationHeader)
	for k, v := range message.Headers {
		ah[k] = []string{v}
	}
	sd := payment.SerializedDetails(message.Body)

	submit, err := h.payment.Submit(ctx, ah, sd)
	if err != nil {
		if isRetry(err) {
			return consumer.NewRetryError(minRetryAfter, err)
		}
		return fmt.Errorf("error calling submit: %w", err)
	}

	if !submit.IsComplete() {
		return consumer.NewRetryError(submit.RetryAfter, ErrSubmitNotComplete)
	}

	return nil
}

func isRetry(err error) bool {
	hs, err := clients.UnwrapHTTPState(err)
	if err != nil {
		return true // To be safe retry all unknown errors.
	}
	_, ok := retryRespCodes[hs.Status]
	return ok
}

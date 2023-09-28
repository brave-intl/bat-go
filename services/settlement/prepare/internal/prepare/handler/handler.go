package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer"
	"github.com/brave-intl/bat-go/services/settlement/internal/payment"
)

var (
	permErrorCodes = map[int]struct{}{
		http.StatusBadRequest:   {},
		http.StatusUnauthorized: {},
		http.StatusForbidden:    {},
	}
)

// Storage defines the methods signatures for storing transactions.
type Storage interface {
	SaveTransaction(ctx context.Context, payoutID string, attestedDetails payment.AttestedDetails) error
}

// PaymentClient defines the methods signatures used to call the payment service.
type PaymentClient interface {
	Prepare(ctx context.Context, details payment.SerializedDetails) (payment.AttestedDetails, error)
}

// Handler represents a prepare message handler.
type Handler struct {
	store      Storage
	payment    PaymentClient
	payoutConf payment.Config
	retry      backoff.RetryFunc
}

// New returns a new instance of prepare handler.
func New(storage Storage, payment PaymentClient, payoutConf payment.Config, retry backoff.RetryFunc) *Handler {
	return &Handler{
		store:      storage,
		payment:    payment,
		payoutConf: payoutConf,
		retry:      retry,
	}
}

// Handle processes prepare transaction messages. Handle calls the payment service prepare endpoint then stores the
// successfully attested transaction response.
func (h *Handler) Handle(ctx context.Context, message consumer.Message) error {
	const (
		minRetryAfter = 1
	)

	sd := payment.SerializedDetails(message.Body)

	attestedDet, err := h.payment.Prepare(ctx, sd)
	if err != nil {
		if isRetry(err) {
			return consumer.NewRetryError(minRetryAfter, err)
		}
		return fmt.Errorf("error calling prepare: %w", err)
	}

	err = h.store.SaveTransaction(ctx, h.payoutConf.PayoutID, attestedDet)
	if err != nil {
		return consumer.NewRetryError(minRetryAfter, err)
	}

	return nil
}

func isRetry(err error) bool {
	hs, err := clients.UnwrapHTTPState(err)
	if err != nil {
		return true
	}
	_, ok := permErrorCodes[hs.Status]
	return !ok // don't retry if it's in the set.
}

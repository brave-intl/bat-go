package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/services/skus/model"
)

type tlv2Svc interface {
	UniqBatches(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error)
}

type Credential struct {
	tlv2 tlv2Svc
}

func NewCredential(tlv2 tlv2Svc) *Credential {
	result := &Credential{tlv2: tlv2}

	return result
}

func (h *Credential) CountBatches(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	ctx := r.Context()

	orderID, err := uuid.FromString(chi.URLParamFromCtx(ctx, "orderID"))
	if err != nil {
		return handlers.ValidationError("request", map[string]interface{}{"orderID": err.Error()})
	}

	lim, nact, err := h.tlv2.UniqBatches(ctx, orderID, uuid.Nil)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return handlers.WrapError(err, "cliend ended request", model.StatusClientClosedConn)

		case errors.Is(err, model.ErrOrderNotFound), errors.Is(err, model.ErrInvalidOrderNoItems), errors.Is(err, model.ErrOrderItemNotFound):
			return handlers.WrapError(err, "order not found", http.StatusNotFound)

		case errors.Is(err, model.ErrOrderNotPaid):
			return handlers.WrapError(err, "order not paid", http.StatusPaymentRequired)

		case errors.Is(err, model.ErrUnsupportedCredType):
			return handlers.WrapError(err, "credential type not supported", http.StatusBadRequest)

		default:
			return handlers.WrapError(model.ErrSomethingWentWrong, "something went wrong", http.StatusInternalServerError)
		}
	}

	result := &struct {
		Limit  int `json:"limit"`
		Active int `json:"active"`
	}{Limit: lim, Active: nact}

	return handlers.RenderContent(ctx, result, w, http.StatusOK)
}

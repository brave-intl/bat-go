package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/services/skus/model"
)

type tlv2Svc interface {
	UniqBatches(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error)
	ListBatches(ctx context.Context, orderID, itemID uuid.UUID) ([]model.TLV2ActiveBatch, error)
	DeleteBatchSeats(ctx context.Context, orderID, itemID uuid.UUID, seats int) error
}

type Cred struct {
	tlv2 tlv2Svc
}

func NewCred(tlv2 tlv2Svc) *Cred {
	result := &Cred{tlv2: tlv2}

	return result
}

func (h *Cred) CountBatches(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	ctx := r.Context()

	orderID, err := uuid.FromString(chi.URLParamFromCtx(ctx, "orderID"))
	if err != nil {
		return handlers.ValidationError("request", map[string]interface{}{"orderID": err.Error()})
	}

	lim, nact, err := h.tlv2.UniqBatches(ctx, orderID, uuid.Nil)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return handlers.WrapError(err, "client ended request", model.StatusClientClosedConn)

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

// ListBatches returns the active credential batches (linked devices) for an order.
// An optional item_id query parameter scopes the results to a specific order item.
//
// GET /v1/orders/{orderID}/credentials/batches
func (h *Cred) ListBatches(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	ctx := r.Context()

	orderID, err := uuid.FromString(chi.URLParamFromCtx(ctx, "orderID"))
	if err != nil {
		return handlers.ValidationError("request", map[string]interface{}{"orderID": err.Error()})
	}

	itemID := uuid.Nil
	if raw := r.URL.Query().Get("item_id"); raw != "" {
		itemID, err = uuid.FromString(raw)
		if err != nil {
			return handlers.ValidationError("request", map[string]interface{}{"item_id": err.Error()})
		}
	}

	batches, err := h.tlv2.ListBatches(ctx, orderID, itemID)
	if err != nil {
		return credBatchError(err)
	}

	if batches == nil {
		batches = []model.TLV2ActiveBatch{}
	}

	result := &struct {
		Batches []model.TLV2ActiveBatch `json:"batches"`
	}{Batches: batches}

	return handlers.RenderContent(ctx, result, w, http.StatusOK)
}

// DeleteBatchSeats frees device linking slots by removing the oldest active credential
// batches for an order. The request body specifies how many seats to free and an
// optional item_id to scope the operation to a specific order item.
//
// DELETE /v1/orders/{orderID}/credentials/batches
func (h *Cred) DeleteBatchSeats(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	ctx := r.Context()

	orderID, err := uuid.FromString(chi.URLParamFromCtx(ctx, "orderID"))
	if err != nil {
		return handlers.ValidationError("request", map[string]interface{}{"orderID": err.Error()})
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		return handlers.WrapError(err, "failed to read request body", http.StatusBadRequest)
	}

	req := &struct {
		Seats  int    `json:"seats"`
		ItemID string `json:"item_id,omitempty"`
	}{}

	if err := json.Unmarshal(body, req); err != nil {
		return handlers.WrapError(err, "failed to parse request body", http.StatusBadRequest)
	}

	if req.Seats <= 0 {
		return handlers.ValidationError("request", map[string]interface{}{"seats": "must be a positive integer"})
	}

	itemID := uuid.Nil
	if req.ItemID != "" {
		itemID, err = uuid.FromString(req.ItemID)
		if err != nil {
			return handlers.ValidationError("request", map[string]interface{}{"item_id": err.Error()})
		}
	}

	if err := h.tlv2.DeleteBatchSeats(ctx, orderID, itemID, req.Seats); err != nil {
		return credBatchError(err)
	}

	return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
}

func credBatchError(err error) *handlers.AppError {
	switch {
	case errors.Is(err, context.Canceled):
		return handlers.WrapError(err, "client ended request", model.StatusClientClosedConn)

	case errors.Is(err, context.DeadlineExceeded):
		return handlers.WrapError(err, "request timed out", http.StatusGatewayTimeout)

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

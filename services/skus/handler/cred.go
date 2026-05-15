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
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/skus/model"
)

type tlv2Svc interface {
	UniqBatches(ctx context.Context, orderID, itemID uuid.UUID) (*model.BatchesStatus, error)
	ListActiveBatches(ctx context.Context, orderID, itemID uuid.UUID) ([]model.TLV2ActiveBatch, error)
	DeleteBatches(ctx context.Context, orderID, itemID uuid.UUID, seats int) error
	ExtendLinkingLimit(ctx context.Context, orderID, itemID uuid.UUID, write model.ExtensionWrite) error
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

	status, err := h.tlv2.UniqBatches(ctx, orderID, uuid.Nil)
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

	return handlers.RenderContent(ctx, status, w, http.StatusOK)
}

// ListActiveBatches returns the active credential batches (linked devices) for an order.
// An optional item_id query parameter scopes the results to a specific order item.
//
// GET /v1/orders/{orderID}/credentials/batches
func (h *Cred) ListActiveBatches(w http.ResponseWriter, r *http.Request) *handlers.AppError {
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

	batches, err := h.tlv2.ListActiveBatches(ctx, orderID, itemID)
	if err != nil {
		lg := logging.Logger(ctx, "skus").With().Str("func", "ListActiveBatches").Logger()

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
			lg.Error().Err(err).Msg("failed to list active batches")
			return handlers.WrapError(model.ErrSomethingWentWrong, "something went wrong", http.StatusInternalServerError)
		}
	}

	if batches == nil {
		batches = []model.TLV2ActiveBatch{}
	}

	result := model.BatchListResp{Batches: batches}

	return handlers.RenderContent(ctx, result, w, http.StatusOK)
}

// DeleteBatches frees device linking slots by removing the oldest active credential
// batches for an order. The request body specifies how many seats to free and an
// optional item_id to scope the operation to a specific order item.
//
// DELETE /v1/orders/{orderID}/credentials/batches
func (h *Cred) DeleteBatches(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	ctx := r.Context()

	orderID, err := uuid.FromString(chi.URLParamFromCtx(ctx, "orderID"))
	if err != nil {
		return handlers.ValidationError("request", map[string]interface{}{"orderID": err.Error()})
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, reqBodyLimit10MB))
	if err != nil {
		return handlers.WrapError(err, "failed to read request body", http.StatusBadRequest)
	}

	var req model.DeleteBatchesReq
	if err := json.Unmarshal(body, &req); err != nil {
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

	if err := h.tlv2.DeleteBatches(ctx, orderID, itemID, req.Seats); err != nil {
		lg := logging.Logger(ctx, "skus").With().Str("func", "DeleteBatches").Logger()

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

		case errors.Is(err, model.ErrBatchSeatsExceeded):
			return handlers.WrapError(err, "seats exceeds active batch count", http.StatusBadRequest)

		default:
			lg.Error().Err(err).Msg("failed to delete batches")
			return handlers.WrapError(model.ErrSomethingWentWrong, "something went wrong", http.StatusInternalServerError)
		}
	}

	return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
}

// POST /v1/orders/{orderID}/credentials/items/{itemID}/batches/extend
func (h *Cred) ExtendLinkingLimit(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	ctx := r.Context()

	orderID, err := uuid.FromString(chi.URLParamFromCtx(ctx, "orderID"))
	if err != nil {
		return handlers.ValidationError("request", map[string]interface{}{"orderID": err.Error()})
	}

	itemID, err := uuid.FromString(chi.URLParamFromCtx(ctx, "itemID"))
	if err != nil {
		return handlers.ValidationError("request", map[string]interface{}{"itemID": err.Error()})
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, reqBodyLimit10MB))
	if err != nil {
		return withErrorCode(handlers.WrapError(err, "failed to read request body", http.StatusBadRequest), model.ExtensionCodeMalformedBody)
	}

	var write model.ExtensionWrite
	if err := json.Unmarshal(body, &write); err != nil {
		return withErrorCode(handlers.WrapError(err, "failed to parse request body", http.StatusBadRequest), model.ExtensionCodeMalformedBody)
	}

	if err := h.tlv2.ExtendLinkingLimit(ctx, orderID, itemID, write); err != nil {
		lg := logging.Logger(ctx, "skus").With().Str("func", "ExtendLinkingLimit").Logger()

		switch {
		case errors.Is(err, context.Canceled):
			return handlers.WrapError(err, "client ended request", model.StatusClientClosedConn)

		case errors.Is(err, context.DeadlineExceeded):
			return handlers.WrapError(err, "request timed out", http.StatusGatewayTimeout)

		case errors.Is(err, model.ErrOrderNotFound), errors.Is(err, model.ErrInvalidOrderNoItems), errors.Is(err, model.ErrOrderItemNotFound):
			return withErrorCode(handlers.WrapError(err, "order not found", http.StatusNotFound), model.ExtensionCodeOrderNotFound)

		case errors.Is(err, model.ErrOrderNotPaid):
			return withErrorCode(handlers.WrapError(err, "order not paid", http.StatusPaymentRequired), model.ExtensionCodeOrderNotPaid)

		case errors.Is(err, model.ErrUnsupportedCredType):
			return withErrorCode(handlers.WrapError(err, "credential type not supported", http.StatusBadRequest), model.ExtensionCodeUnsupportedCredType)

		case errors.Is(err, model.ErrExtensionInvalidLimit):
			return withErrorCode(handlers.WrapError(err, "extension new limit invalid", http.StatusUnprocessableEntity), model.ExtensionCodeInvalidLimit)

		case errors.Is(err, model.ErrExtensionConflict):
			return withErrorCode(handlers.WrapError(err, "extension version conflict", http.StatusConflict), model.ExtensionCodeConflict)

		default:
			lg.Error().Err(err).Msg("failed to extend linking limit")
			return handlers.WrapError(model.ErrSomethingWentWrong, "something went wrong", http.StatusInternalServerError)
		}
	}

	return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
}

func withErrorCode(appErr *handlers.AppError, code string) *handlers.AppError {
	appErr.ErrorCode = code
	return appErr
}

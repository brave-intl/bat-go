package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi"
	"github.com/go-playground/validator/v10"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/inputs"

	"github.com/brave-intl/bat-go/services/skus/model"
)

type credService interface {
	CreateOrderItemCredentials(ctx context.Context, orderID, itemID, reqID uuid.UUID, creds []string) error
	CredsForItem(ctx context.Context, orderID, itemID, reqID uuid.UUID) (interface{}, int, error)

	GetOutboxMovAvgDurationSeconds(ctx context.Context) (int64, error)
}

type Credentials struct {
	svc   credService
	valid *validator.Validate
}

func NewCredentials(svc credService) *Credentials {
	result := &Credentials{
		svc:   svc,
		valid: validator.New(),
	}

	return result
}

func (h *Credentials) CreateForItem(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	ctx := r.Context()

	orderID := &inputs.ID{}
	if err := inputs.DecodeAndValidateString(ctx, orderID, chi.URLParam(r, "orderID")); err != nil {
		return handlers.ValidationError("Error validating request url parameter", map[string]interface{}{
			"orderID": err.Error(),
		})
	}

	itemID := &inputs.ID{}
	if err := inputs.DecodeAndValidateString(ctx, itemID, chi.URLParam(r, "itemID")); err != nil {
		return handlers.ValidationError("Error validating request url parameter", map[string]interface{}{
			"itemID": err.Error(),
		})
	}

	reqID := &inputs.ID{}
	if err := inputs.DecodeAndValidateString(ctx, reqID, chi.URLParam(r, "requestID")); err != nil {
		return handlers.ValidationError("Error validating request url parameter", map[string]interface{}{
			"requestID": err.Error(),
		})
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
	if err != nil {
		return handlers.WrapError(err, "Failed to read request body", http.StatusBadRequest)
	}

	req := &model.CreateItemCredsRequest{}
	if err := json.Unmarshal(raw, req); err != nil {
		return handlers.WrapError(err, "Failed to deserialize request", http.StatusBadRequest)
	}

	if err := h.valid.StructCtx(ctx, req); err != nil {
		verrs, ok := collectValidationErrors(err)
		if !ok {
			return handlers.WrapError(err, "Failed to validate request", http.StatusBadRequest)
		}

		return &handlers.AppError{
			Message: "Validation failed",
			Code:    http.StatusBadRequest,
			Data:    map[string]interface{}{"validationErrors": verrs},
		}
	}

	if err := h.svc.CreateOrderItemCredentials(ctx, *orderID.UUID(), *itemID.UUID(), *reqID.UUID(), req.BlindedCreds); err != nil {
		return handlers.WrapError(err, "Error creating creds for item", http.StatusInternalServerError)
	}

	return handlers.RenderContent(ctx, nil, w, http.StatusOK)
}

func (h *Credentials) GetForItem(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	ctx := r.Context()

	orderID := &inputs.ID{}
	if err := inputs.DecodeAndValidateString(ctx, orderID, chi.URLParam(r, "orderID")); err != nil {
		return handlers.ValidationError("Error validating request url parameter", map[string]interface{}{
			"orderID": err.Error(),
		})
	}

	itemID := &inputs.ID{}
	if err := inputs.DecodeAndValidateString(ctx, itemID, chi.URLParam(r, "itemID")); err != nil {
		return handlers.ValidationError("Error validating request url parameter", map[string]interface{}{
			"itemID": err.Error(),
		})
	}

	reqID := &inputs.ID{}
	if err := inputs.DecodeAndValidateString(ctx, reqID, chi.URLParam(r, "requestID")); err != nil {
		return handlers.ValidationError("Error validating request url parameter", map[string]interface{}{
			"requestID": err.Error(),
		})
	}

	creds, status, err := h.svc.CredsForItem(ctx, *orderID.UUID(), *itemID.UUID(), *reqID.UUID())
	if err != nil {
		if !errors.Is(err, model.ErrSetRetryAfter) {
			return handlers.WrapError(err, "Error getting credentials", status)
		}

		// Handle a retry-after error: add it to response header.
		avg, err := h.svc.GetOutboxMovAvgDurationSeconds(ctx)
		if err != nil {
			return handlers.WrapError(err, "Error getting credential retry-after", status)
		}

		w.Header().Set("Retry-After", strconv.FormatInt(avg, 10))
	}

	if creds == nil {
		return handlers.RenderContent(ctx, map[string]interface{}{}, w, status)
	}

	return handlers.RenderContent(ctx, creds, w, status)
}

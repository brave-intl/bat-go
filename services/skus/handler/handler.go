package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/asaskevich/govalidator"
	"github.com/go-chi/chi"
	"github.com/go-playground/validator/v10"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/requestutils"

	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	reqBodyLimit10MB = 10 << 20
)

type orderService interface {
	CreateOrderFromRequest(ctx context.Context, req model.CreateOrderRequest) (*model.Order, error)
	CreateOrder(ctx context.Context, req *model.CreateOrderRequestNew) (*model.Order, error)
	CancelOrder(ctx context.Context, id uuid.UUID) error
}

type Order struct {
	svc   orderService
	valid *validator.Validate
}

func NewOrder(svc orderService) *Order {
	result := &Order{
		svc:   svc,
		valid: validator.New(),
	}

	return result
}

func (h *Order) Create(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	ctx := r.Context()

	var req model.CreateOrderRequest
	if err := requestutils.ReadJSON(ctx, r.Body, &req); err != nil {
		return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
	}

	if _, err := govalidator.ValidateStruct(req); err != nil {
		return handlers.WrapValidationError(err)
	}

	if len(req.Items) == 0 {
		return handlers.ValidationError("request body", map[string]interface{}{"items": "array must contain at least one item"})
	}

	lg := logging.Logger(ctx, "skus").With().Str("func", "CreateOrderHandler").Logger()

	// The SKU is validated in CreateOrderItemFromMacaroon.
	order, err := h.svc.CreateOrderFromRequest(ctx, req)
	if err != nil {
		if errors.Is(err, model.ErrInvalidSKU) {
			lg.Err(err).Msg("invalid sku")

			return handlers.ValidationError(err.Error(), nil)
		}

		lg.Err(err).Msg("error creating the order")

		return handlers.WrapError(err, "Error creating the order in the database", http.StatusInternalServerError)
	}

	return handlers.RenderContent(ctx, order, w, http.StatusCreated)
}

func (h *Order) CreateNew(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	raw, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
	if err != nil {
		return handlers.WrapError(err, "Failed to read request body", http.StatusBadRequest)
	}

	req := &model.CreateOrderRequestNew{}
	if err := json.Unmarshal(raw, req); err != nil {
		return handlers.WrapError(err, "Failed to deserialize request", http.StatusBadRequest)
	}

	ctx := r.Context()

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

	lg := logging.Logger(ctx, "skus").With().Str("func", "CreateOrderNew").Logger()

	result, err := h.svc.CreateOrder(ctx, req)
	if err != nil {
		lg.Err(err).Msg("failed to create order")

		if errors.Is(err, model.ErrInvalidOrderRequest) {
			return handlers.WrapError(err, "Invalid order data supplied", http.StatusUnprocessableEntity)
		}

		return handlers.WrapError(model.ErrSomethingWentWrong, "Couldn't finish creating order", http.StatusInternalServerError)
	}

	return handlers.RenderContent(ctx, result, w, http.StatusCreated)
}

func (h *Order) Cancel(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	ctx := r.Context()

	orderID, err := uuid.FromString(chi.URLParamFromCtx(ctx, "orderID"))
	if err != nil {
		return handlers.ValidationError("request", map[string]interface{}{"orderID": model.ErrInvalidUUID})
	}

	lg := logging.Logger(ctx, "skus").With().Str("func", "CancelOrderNew").Logger()

	if err := h.svc.CancelOrder(ctx, orderID); err != nil {
		lg.Err(err).Str("order_id", orderID.String()).Msg("failed to cancel order")

		if errors.Is(err, context.Canceled) {
			return handlers.WrapError(model.ErrSomethingWentWrong, "client ended request", model.StatusClientClosedConn)
		}

		return handlers.WrapError(model.ErrSomethingWentWrong, "could not cancel order", http.StatusInternalServerError)
	}

	return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
}

func collectValidationErrors(err error) (map[string]string, bool) {
	var verr validator.ValidationErrors
	if !errors.As(err, &verr) {
		return nil, false
	}

	result := make(map[string]string, len(verr))
	for i := range verr {
		result[verr[i].Field()] = verr[i].Error()
	}

	return result, true
}

package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/asaskevich/govalidator"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/requestutils"

	"github.com/brave-intl/bat-go/services/skus/model"
)

type orderService interface {
	CreateOrderFromRequest(ctx context.Context, req model.CreateOrderRequest) (*model.Order, error)
}

type Order struct {
	svc orderService
}

func NewOrder(svc orderService) *Order {
	result := &Order{
		svc: svc,
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
		return handlers.ValidationError(
			"Error validating request body",
			map[string]interface{}{
				"items": "array must contain at least one item",
			},
		)
	}

	lg := logging.Logger(ctx, "payments").With().Str("func", "CreateOrderHandler").Logger()

	// The SKU is validated in CreateOrderItemFromMacaroon.
	order, err := h.svc.CreateOrderFromRequest(ctx, req)
	if err != nil {
		if errors.Is(err, model.ErrInvalidSKU) {
			lg.Error().Err(err).Msg("invalid sku")
			return handlers.ValidationError(err.Error(), nil)
		}

		lg.Error().Err(err).Msg("error creating the order")
		return handlers.WrapError(err, "Error creating the order in the database", http.StatusInternalServerError)
	}

	return handlers.RenderContent(ctx, order, w, http.StatusCreated)
}

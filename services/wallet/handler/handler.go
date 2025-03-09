package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/inputs"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/services/wallet/model"
)

const (
	reqBodyLimit10MB = 10 << 20
)

type solanaService interface {
	SolanaDeleteFromWaitlist(ctx context.Context, paymentID uuid.UUID) error
	SolanaAddToWaitlist(ctx context.Context, paymentID uuid.UUID) error
}

type Solana struct {
	svc solanaService
}

func NewSolana(solSvc solanaService) *Solana {
	return &Solana{svc: solSvc}
}

type waitlistRequest struct {
	PaymentID uuid.UUID `json:"paymentId" valid:"required"`
}

func (h *Solana) PostWaitlist(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	ctx := r.Context()

	lg := logging.Logger(ctx, "handler").With().Str("func", "Solana_PostWaitlist").Logger()

	b, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
	if err != nil {
		lg.Err(err).Msg("failed to read body")

		return handlers.WrapError(err, "error reading body", http.StatusBadRequest)
	}

	var req waitlistRequest
	if err := json.Unmarshal(b, &req); err != nil {
		lg.Err(err).Msg("failed to parse ")

		return handlers.WrapError(err, "error decoding body", http.StatusBadRequest)
	}

	if _, err := govalidator.ValidateStruct(req); err != nil {
		lg.Err(err).Msg("failed to validate req")

		return handlers.WrapValidationError(err)
	}

	sk, err := middleware.GetKeyID(ctx)
	if err != nil {
		lg.Err(err).Msg("failed to get key id")

		return handlers.ValidationError("request body", map[string]interface{}{"paymentId": err.Error()})
	}

	if sk != req.PaymentID.String() {
		lg.Err(model.ErrPaymentIDSignatureMismatch).Msg("paymentId signature mismatch")

		return handlers.ValidationError("request body", map[string]interface{}{"paymentId": model.ErrPaymentIDSignatureMismatch})
	}

	if err := h.svc.SolanaAddToWaitlist(ctx, req.PaymentID); err != nil {
		lg.Err(err).Msg("failed to add payment id to solana waitlist")

		switch {
		case errors.Is(err, model.ErrSolAlreadyLinked):
			return handlers.WrapError(model.ErrSolAlreadyLinked, "already linked to solana", http.StatusBadRequest)

		default:
			return handlers.WrapError(model.ErrInternalServer, "internal server error", http.StatusInternalServerError)
		}
	}

	return handlers.RenderContent(ctx, struct{}{}, w, http.StatusCreated)
}

func (h *Solana) DeleteWaitlist(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	ctx := r.Context()

	lg := logging.Logger(ctx, "handler").With().Str("func", "Solana_DeleteWaitlist").Logger()

	var paymentID inputs.ID
	if err := inputs.DecodeAndValidateString(ctx, &paymentID, chi.URLParam(r, "paymentID")); err != nil {
		return handlers.WrapError(err, "invalid paymentID", http.StatusBadRequest)
	}

	sk, err := middleware.GetKeyID(ctx)
	if err != nil {
		lg.Err(err).Msg("failed to get key id")

		return handlers.ValidationError("request", map[string]interface{}{"paymentID": err.Error()})
	}

	if sk != paymentID.String() {
		lg.Err(model.ErrPaymentIDSignatureMismatch).Msg("paymentId signature mismatch")

		return handlers.ValidationError("request", map[string]interface{}{"paymentID": model.ErrPaymentIDSignatureMismatch})
	}

	if err := h.svc.SolanaDeleteFromWaitlist(ctx, *paymentID.UUID()); err != nil {
		lg.Err(err).Msg("failed to add payment id to solana waitlist")

		return handlers.WrapError(model.ErrInternalServer, "internal server error", http.StatusInternalServerError)
	}

	return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
}

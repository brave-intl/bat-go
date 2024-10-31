package handler

import (
	"context"
	"net/http"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/rewards"
	"github.com/brave-intl/bat-go/services/rewards/model"
)

type cardService interface {
	GetCardsAsBytes(ctx context.Context) (rewards.CardBytes, error)
}

type CardsHandler struct {
	cardSvc cardService
}

func NewCardsHandler(cardSvc cardService) *CardsHandler {
	return &CardsHandler{
		cardSvc: cardSvc,
	}
}

const errSomethingWentWrong model.Error = "something went wrong"

func (c *CardsHandler) GetCardsHandler(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	ctx := r.Context()

	l := logging.Logger(ctx, "handler").With().Str("func", "GetCardsHandler").Logger()

	cards, err := c.cardSvc.GetCardsAsBytes(ctx)
	if err != nil {
		l.Err(err).Msg("failed to get cards as bytes")

		return handlers.WrapError(errSomethingWentWrong, errSomethingWentWrong.Error(), http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)

	w.Header().Set("Content-Type", "application/json")

	if _, err := w.Write(cards); err != nil {
		l.Err(err).Msg("failed to write response")

		return handlers.WrapError(errSomethingWentWrong, errSomethingWentWrong.Error(), http.StatusInternalServerError)
	}

	return nil
}

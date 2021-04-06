package eyeshade

import (
	"net/http"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/go-chi/chi"
)

// RouterStats holds routes having to do with collecting stats on transactions
func (service *Service) RouterStats() chi.Router {
	r := chi.NewRouter()
	scopes := []string{"stats", "global"}
	r.Method("GET", "/grants/{type}/{start}/{until}", middleware.InstrumentHandler(
		"GETGrantStats",
		middleware.SimpleScopedTokenAuthorizedOnly(
			service.GETGrantStats(),
			scopes,
		),
	))
	r.Method("GET", "/settlements/{type}/{start}/{until}", middleware.InstrumentHandler(
		"GETSettlementStats",
		middleware.SimpleScopedTokenAuthorizedOnly(
			service.GETSettlementStats(),
			scopes,
		),
	))
	return r
}

// GETSettlementStats retrieves settlement stats
func (service *Service) GETSettlementStats() handlers.AppHandler {
	return handlers.AppHandler(func(
		w http.ResponseWriter,
		r *http.Request,
	) *handlers.AppError {
		options, err := models.NewSettlementStatOptions(
			chi.URLParam(r, "type"),
			chi.URLParam(r, ""),
			time.RFC3339,
			chi.URLParam(r, "start"),
			chi.URLParam(r, "until"),
		)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		body, err := service.GetSettlementStats(r.Context(), *options)
		if err != nil {
			return handlers.WrapError(err, "unable to get referral groups")
		}
		return handlers.RenderContent(r.Context(), body, w, http.StatusOK)
	})
}

// GETGrantStats retrieves grant stats
func (service *Service) GETGrantStats() handlers.AppHandler {
	return handlers.AppHandler(func(
		w http.ResponseWriter,
		r *http.Request,
	) *handlers.AppError {
		options, err := models.NewGrantStatOptions(
			chi.URLParam(r, "type"),
			time.RFC3339,
			chi.URLParam(r, "start"),
			chi.URLParam(r, "until"),
		)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		body, err := service.GetGrantStats(r.Context(), *options)
		if err != nil {
			return handlers.WrapError(err, "unable to get referral groups")
		}
		return handlers.RenderContent(r.Context(), body, w, http.StatusOK)
	})
}

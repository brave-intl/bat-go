package eyeshade

import (
	"net/http"

	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	requestutils "github.com/brave-intl/bat-go/utils/request"
	"github.com/go-chi/chi"
)

// RouterSettlements holds routes having to do with collecting stats on transactions
func (service *Service) RouterSettlements() chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/settlement", middleware.InstrumentHandler(
		"SettlementsGrantsBounded",
		middleware.SimpleScopedTokenAuthorizedOnly(
			service.POSTSettlements(),
			"publishers",
		),
	))
	return r
}

// POSTSettlements handles settlements
func (service *Service) POSTSettlements() handlers.AppHandler {
	return handlers.AppHandler(func(
		w http.ResponseWriter,
		r *http.Request,
	) *handlers.AppError {
		var messages []models.Settlement
		err := requestutils.ReadJSON(r.Body, &messages)
		if err != nil {
			return handlers.WrapValidationError(err)
		}
		err = service.ProduceSettlements(r.Context(), messages)
		if err != nil {
			return handlers.WrapError(err, "unable to produce messages", http.StatusInternalServerError)
		}
		return handlers.RenderContent(r.Context(), nil, w)
	})
}

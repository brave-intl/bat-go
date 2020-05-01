package rewards

import (
	"net/http"

	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/go-chi/chi"
)

// Router for rewards endpoints
func Router(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/parameters", GetParameters(service))
	return r
}

// GetParameters - handler to get reward parameters
func GetParameters(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// in here we need to validate our currency

		var parameters, err = service.GetParameters(r.Context())
		if err != nil {
			return handlers.WrapError(err, "Error create api keys", http.StatusInternalServerError)
		}
		return handlers.RenderContent(r.Context(), parameters, w, http.StatusOK)
	})
}

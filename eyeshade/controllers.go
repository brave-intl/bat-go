package eyeshade

import (
	"net/http"

	"github.com/brave-intl/bat-go/utils/handlers"
)

// EndpointNotImplemented a placeholder for not implemented endpoints
func (service *Service) EndpointNotImplemented() handlers.AppHandler {
	return handlers.AppHandler(func(
		w http.ResponseWriter,
		r *http.Request,
	) *handlers.AppError {
		body := struct {
			Payload string `json:"payload"`
		}{
			Payload: "not implemented",
		}
		return handlers.RenderContent(r.Context(), body, w, http.StatusNotImplemented)
	})
}

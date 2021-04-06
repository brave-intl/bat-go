package eyeshade

import (
	"github.com/brave-intl/bat-go/middleware"
	"github.com/go-chi/chi"
)

// RouterSettlements holds routes having to do with collecting stats on transactions
func (service *Service) RouterSettlements() chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/settlement", middleware.InstrumentHandler(
		"SettlementsGrantsBounded",
		service.EndpointNotImplemented(),
	))
	return r
}

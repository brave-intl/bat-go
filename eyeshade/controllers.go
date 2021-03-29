package eyeshade

import (
	"net/http"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/go-chi/chi"
)

type defunctRoute struct {
	method string
	path   string
}

var (
	defunctRoutes = []defunctRoute{
		{"POST", "/v2/publishers/settlement/submit"},
		{"GET", "/v1/referrals/statement/{owner}"},
		{"PUT", "/v1/referrals/{transactionID}"},
		{"POST", "/v1/snapshots/"},
		{"GET", "/v1/snapshots/{snapshotID}"},
		{"GET", "/v1/referrals/{transactionID}"},
		// global
		{"GET", "/v1/login"},
		{"POST", "/v1/login"},
		{"GET", "/v1/logout"},
		{"GET", "/v1/ping"},
	}
)

// DefunctRouter for defunct eyeshade endpoints
func DefunctRouter() chi.Router {
	r := chi.NewRouter()
	for _, routeSettings := range defunctRoutes {
		r.Method(
			routeSettings.method,
			routeSettings.path,
			handlers.AppHandler(
				func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
					return handlers.RenderContent(r.Context(), nil, w, http.StatusGone)
				},
			),
		)
	}
	return r
}

// ReferralsRouter returns information on referral groups
func ReferralsRouter(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/groups", middleware.InstrumentHandler(
		"ReferralGroups",
		NotImplemented(service),
	))
	return r
}

// StatsRouter holds routes having to do with collecting stats on transactions
func StatsRouter(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/grants/{type}/{start}/{until}", middleware.InstrumentHandler(
		"StatsGrantsBounded",
		NotImplemented(service),
	))
	r.Method("GET", "/settlements/{type}/{start}/{until}", middleware.InstrumentHandler(
		"StatsSettlementBounded",
		NotImplemented(service),
	))
	return r
}

// SettlementsRouter holds routes having to do with collecting stats on transactions
func SettlementsRouter(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/settlement", middleware.InstrumentHandler(
		"SettlementsGrantsBounded",
		NotImplemented(service),
	))
	return r
}

// NotImplemented a placeholder for not implemented endpoints
func NotImplemented(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(
		w http.ResponseWriter,
		r *http.Request,
	) *handlers.AppError {
		return handlers.RenderContent(r.Context(), struct {
			Payload string `json:"payload"`
		}{
			Payload: "not yet implemented",
		}, w, http.StatusOK)
	})
}

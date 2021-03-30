package eyeshade

import (
	"net/http"
	"strings"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/go-chi/chi"
)

type defunctRoute struct {
	Method string
	Path   string
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

// DefunctResponse holds a defunct handlers' response
type DefunctResponse struct {
	StatusCode int    `json:"statusCode"`
	Error      string `json:"error"`
	Message    string `json:"message"`
}

// DefunctRouter for defunct eyeshade endpoints
func DefunctRouter(withV1 bool) chi.Router {
	r := chi.NewRouter()
	body := DefunctResponse{
		StatusCode: http.StatusGone,
		Error:      "Gone",
		Message:    "Gone",
	}
	for _, routeSettings := range defunctRoutes {
		path := routeSettings.Path
		isV1 := strings.Contains(path, "/v1/")
		isAndWithV1 := withV1 && isV1
		if isAndWithV1 || (!withV1 && !isV1) {
			if isAndWithV1 {
				path = path[3:]
			}
			r.Method(
				routeSettings.Method,
				path,
				handlers.AppHandler(
					func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
						return handlers.RenderContent(r.Context(), body, w, http.StatusGone)
					},
				),
			)
		}
	}
	return r
}

// ReferralsRouter returns information on referral groups
func (service *Service) ReferralsRouter() chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/groups", middleware.InstrumentHandler(
		"ReferralGroups",
		service.EndpointNotImplemented(),
	))
	return r
}

// StatsRouter holds routes having to do with collecting stats on transactions
func (service *Service) StatsRouter() chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/grants/{type}/{start}/{until}", middleware.InstrumentHandler(
		"StatsGrantsBounded",
		service.EndpointNotImplemented(),
	))
	r.Method("GET", "/settlements/{type}/{start}/{until}", middleware.InstrumentHandler(
		"StatsSettlementBounded",
		service.EndpointNotImplemented(),
	))
	return r
}

// SettlementsRouter holds routes having to do with collecting stats on transactions
func (service *Service) SettlementsRouter() chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/settlement", middleware.InstrumentHandler(
		"SettlementsGrantsBounded",
		service.EndpointNotImplemented(),
	))
	return r
}

// EndpointNotImplemented a placeholder for not implemented endpoints
func (service *Service) EndpointNotImplemented() handlers.AppHandler {
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

package eyeshade

import (
	"net/http"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/go-chi/chi"
	"github.com/maikelmclauflin/go-boom"
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

// RouterDefunct for defunct eyeshade endpoints
func RouterDefunct(withV1 bool) chi.Router {
	r := chi.NewRouter()
	for _, routeSettings := range defunctRoutes {
		path := routeSettings.Path
		isV1 := strings.Contains(path, "/v1/")
		isAndWithV1 := withV1 && isV1
		if !isAndWithV1 && !(!withV1 && !isV1) {
			continue
		}
		if isAndWithV1 {
			path = path[3:]
		}
		r.Method(
			routeSettings.Method,
			path,
			handlers.AppHandler(
				func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
					err := boom.Gone()
					return handlers.RenderContent(
						r.Context(),
						err,
						w,
						err.StatusCode,
					)
				},
			),
		)
	}
	return r
}

// RouterReferrals returns information on referral groups
func (service *Service) RouterReferrals() chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/groups", middleware.InstrumentHandler(
		"ReferralGroups",
		middleware.SimpleScopedTokenAuthorizedOnly(
			service.GETReferralGroups(),
			[]string{"referrals"},
		),
	))
	return r
}

// RouterStats holds routes having to do with collecting stats on transactions
func (service *Service) RouterStats() chi.Router {
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

// RouterSettlements holds routes having to do with collecting stats on transactions
func (service *Service) RouterSettlements() chi.Router {
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
		body := struct {
			Payload string `json:"payload"`
		}{
			Payload: "not yet implemented",
		}
		return handlers.RenderContent(r.Context(), body, w, http.StatusOK)
	})
}

// GETReferralGroups retrieves referral groups
func (service *Service) GETReferralGroups() handlers.AppHandler {
	return handlers.AppHandler(func(
		w http.ResponseWriter,
		r *http.Request,
	) *handlers.AppError {
		query := r.URL.Query()
		resolve := query.Get("resolve") == "true"
		activeAt := inputs.NewTime(time.RFC3339, time.Now())
		_ = inputs.DecodeAndValidateString(r.Context(), activeAt, query.Get("activeAt"))
		fields := requestutils.ManyQueryParams(query["fields"])

		body, err := service.GetReferralGroups(r.Context(), resolve, *activeAt, fields...)
		if err != nil {
			return handlers.WrapError(err, "unable to get referral groups")
		}
		return handlers.RenderContent(r.Context(), body, w, http.StatusOK)
	})
}

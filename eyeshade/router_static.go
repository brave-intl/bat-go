package eyeshade

import (
	"net/http"
	"strings"

	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/go-chi/chi"
	"github.com/maikelmclauflin/go-boom"
)

type defunctRoute struct {
	Method string
	Path   string
}

var (
	DefunctRoutes = []defunctRoute{
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
	for _, routeSettings := range DefunctRoutes {
		path := routeSettings.Path
		isV1 := strings.Contains(path, "/v1/")
		isAndWithV1 := withV1 && isV1
		if !isAndWithV1 && !(!withV1 && !isV1) {
			continue
		}
		if isAndWithV1 {
			path = path[3:]
		}
		r.Method(routeSettings.Method, path,
			handlers.AppHandler(
				func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
					err := boom.Gone()
					return handlers.RenderContent(r.Context(), err, w, err.StatusCode)
				},
			),
		)
	}
	return r
}

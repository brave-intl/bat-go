package eyeshade

import (
	"net/http"
	"time"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
	requestutils "github.com/brave-intl/bat-go/utils/request"
	"github.com/go-chi/chi"
)

// RouterReferrals returns information on referral groups
func (service *Service) RouterReferrals() chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/groups", middleware.InstrumentHandler(
		"ReferralGroups",
		middleware.SimpleScopedTokenAuthorizedOnly(
			service.GETReferralGroups(),
			"referrals",
		),
	))
	return r
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
		fields := append([]string{"id"}, requestutils.ManyQueryParams(query["fields"])...)

		body, err := service.GetReferralGroups(
			r.Context(),
			resolve,
			*activeAt.Time(),
			fields...,
		)
		if err != nil {
			return handlers.WrapError(err, "unable to get referral groups")
		}
		return handlers.RenderContent(r.Context(), body, w, http.StatusOK)
	})
}

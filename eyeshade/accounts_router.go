package eyeshade

import (
	"net/http"
	"strconv"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/go-chi/chi"
)

// AccountsRouter has all information on account info
func AccountsRouter(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/earnings/{type}/total", middleware.InstrumentHandler(
		"AccountEarningsTotal",
		AccountEarningsTotal(service),
	))
	r.Method("GET", "/settlements/{type}/total", middleware.InstrumentHandler(
		"AccountSettlementsTotal",
		NotImplemented(service),
	))
	r.Method("GET", "/balances/{type}/top", middleware.InstrumentHandler(
		"AccountBalancesTop",
		NotImplemented(service),
	))
	r.Method("GET", "/balances", middleware.InstrumentHandler(
		"AccountBalances",
		NotImplemented(service),
	))
	r.Method("GET", "/{account}/transactions", middleware.InstrumentHandler(
		"AccountTransactions",
		NotImplemented(service),
	))
	return r
}

func AccountEarningsTotal(service *Service) handlers.AppHandler {
	validTypes := map[string]bool{
		"contributions": true,
		"referrals":     true,
	}
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		txType := chi.URLParam(r, "type")
		if !validTypes[txType] {
			return handlers.ValidationError(
				"Error validating request path",
				map[string]interface{}{
					"type": "`type` portion of path must either be `contributions` or `referrals`",
				},
			)
		}
		query := r.URL.Query()
		limitParsed, err := strconv.ParseInt(query.Get("limit"), 10, 64)
		var limit *int64
		if err == nil {
			limit = &limitParsed
		}
		earnings, err := service.AccountEarnings(
			r.Context(),
			AccountEarningsOptions{
				Type:      txType,
				Ascending: query.Get("order") == "asc",
				Limit:     limit,
			},
		)
		if err != nil {
			return handlers.WrapError(err, "unable to check account earnings", 0)
		}
		return handlers.RenderContent(r.Context(), earnings, w, http.StatusOK)
	})
}

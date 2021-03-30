package eyeshade

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/go-chi/chi"
)

// AccountsRouter has all information on account info
func (service *Service) AccountsRouter() chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/earnings/{type}/total", middleware.InstrumentHandler(
		"AccountEarningsTotal",
		service.AccountEarningsTotal(),
	))
	r.Method("GET", "/settlements/{type}/total", middleware.InstrumentHandler(
		"AccountSettlementEarningsTotal",
		service.AccountSettlementEarningsTotal(),
	))
	r.Method("GET", "/balances/{type}/top", middleware.InstrumentHandler(
		"AccountBalancesTop",
		service.EndpointNotImplemented(),
	))
	r.Method("GET", "/balances", middleware.InstrumentHandler(
		"AccountBalances",
		service.EndpointNotImplemented(),
	))
	r.Method("GET", "/{account}/transactions", middleware.InstrumentHandler(
		"AccountTransactions",
		service.EndpointNotImplemented(),
	))
	return r
}

// AccountEarningsTotal retrieves the earnings of a limited number of accounts
func (service *Service) AccountEarningsTotal() handlers.AppHandler {
	validTypes := map[string]bool{
		"contributions": true,
		"referrals":     true,
	}
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		txType := chi.URLParam(r, "type")
		if !validTypes[txType] {
			return handlers.ValidationError(
				"request path",
				map[string]interface{}{
					"type": "portion of path must either be `contributions` or `referrals`",
				},
			)
		}
		query := r.URL.Query()
		limitParsed64, err := strconv.ParseInt(query.Get("limit"), 10, 64)
		limit := 100
		if err == nil {
			limit = int(limitParsed64)
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
			status := http.StatusBadRequest
			if !errors.Is(err, ErrLimitReached) && !errors.Is(err, ErrLimitRequired) {
				status = http.StatusInternalServerError
			}
			return handlers.WrapError(err, "unable to check account earnings", status)
		}
		return handlers.RenderContent(r.Context(), earnings, w, http.StatusOK)
	})
}

// AccountSettlementEarningsTotal retrieves the earnings of a limited number of accounts
func (service *Service) AccountSettlementEarningsTotal() handlers.AppHandler {
	validTypes := map[string]bool{
		"contributions": true,
		"referrals":     true,
	}
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		txType := chi.URLParam(r, "type")
		if !validTypes[txType] {
			return handlers.ValidationError(
				"request path",
				map[string]interface{}{
					"type": "portion of path must either be `contributions` or `referrals`",
				},
			)
		}
		query := r.URL.Query()
		limitParsed64, err := strconv.ParseInt(query.Get("limit"), 10, 64)
		limit := 100
		if err == nil {
			limit = int(limitParsed64)
		}
		// dates are optional
		startDateInput := inputs.NewTime("2020-01-01")
		inputs.DecodeAndValidateString(
			r.Context(),
			startDateInput,
			query.Get("start"),
		)

		untilDateInput := inputs.NewTime("2020-01-01")
		inputs.DecodeAndValidateString(
			r.Context(),
			untilDateInput,
			query.Get("start"),
		)
		paid, err := service.AccountSettlementEarnings(
			r.Context(),
			AccountSettlementEarningsOptions{
				Type:      txType,
				Ascending: query.Get("order") == "asc",
				Limit:     limit,
				StartDate: startDateInput.Time(),
				UntilDate: untilDateInput.Time(),
			},
		)
		if err != nil {
			status := http.StatusBadRequest
			if !errors.Is(err, ErrLimitReached) && !errors.Is(err, ErrLimitRequired) {
				status = http.StatusInternalServerError
			}
			return handlers.WrapError(err, "unable to check account settlement earnings", status)
		}
		return handlers.RenderContent(r.Context(), paid, w, http.StatusOK)
	})
}

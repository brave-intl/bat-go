package eyeshade

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
	requestutils "github.com/brave-intl/bat-go/utils/request"
	"github.com/go-chi/chi"
)

var (
	validTransactionTypes = map[string]bool{
		"contributions": true,
		"referrals":     true,
	}
)

// RouterAccounts has all information on account info
func (service *Service) RouterAccounts() chi.Router {
	r := chi.NewRouter()
	scopes := []string{"publishers"}
	r.Method("GET", "/earnings/{type}/total", middleware.InstrumentHandler(
		"GETAccountEarningsTotal",
		middleware.SimpleScopedTokenAuthorizedOnly(
			service.GETAccountEarningsTotal(),
			scopes...,
		),
	))
	r.Method("GET", "/settlements/{type}/total", middleware.InstrumentHandler(
		"GETAccountSettlementEarningsTotal",
		middleware.SimpleScopedTokenAuthorizedOnly(
			service.GETAccountSettlementEarningsTotal(),
			scopes...,
		),
	))
	r.Method("GET", "/balances", middleware.InstrumentHandler(
		"GETAccountBalances",
		middleware.SimpleScopedTokenAuthorizedOnly(
			service.GETBalances(),
			scopes...,
		),
	))
	r.Method("GET", "/{account}/transactions", middleware.InstrumentHandler(
		"GETAccountTransactions",
		middleware.SimpleScopedTokenAuthorizedOnly(
			service.GETTransactionsByAccount(),
			scopes...,
		),
	))
	return r
}

// GETAccountEarningsTotal retrieves the earnings of a limited number of accounts
func (service *Service) GETAccountEarningsTotal() handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		txType := chi.URLParam(r, "type")
		if !validTransactionTypes[txType] {
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
		earnings, err := service.GetAccountEarnings(
			r.Context(),
			models.AccountEarningsOptions{
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

// GETAccountSettlementEarningsTotal retrieves the earnings of a limited number of accounts
func (service *Service) GETAccountSettlementEarningsTotal() handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		txType := chi.URLParam(r, "type")
		if !validTransactionTypes[txType] {
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
		startDateInput := inputs.NewTime(time.RFC3339)
		_ = inputs.DecodeAndValidateString(
			r.Context(),
			startDateInput,
			query.Get("start"),
		)

		untilDateInput := inputs.NewTime(time.RFC3339)
		_ = inputs.DecodeAndValidateString(
			r.Context(),
			untilDateInput,
			query.Get("until"),
		)
		paid, err := service.GetAccountSettlementEarnings(
			r.Context(),
			models.AccountSettlementEarningsOptions{
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

// GETBalances retrieves the balances for a given set of identifiers
func (service *Service) GETBalances() handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		accountIDs := r.URL.Query()["account"] // access map directly to get all items
		includePending := r.URL.Query().Get("pending") == "true"
		balances, err := service.GetBalances(r.Context(), accountIDs, includePending)
		if err != nil {
			return handlers.WrapError(err, "unable to get balances for account earnings", http.StatusBadRequest)
		}
		return handlers.RenderContent(r.Context(), balances, w, http.StatusOK)
	})
}

// GETTransactionsByAccount retrieves the transactions for a given set of identifiers
func (service *Service) GETTransactionsByAccount() handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		accountID := chi.URLParam(r, "account")
		txTypes := requestutils.ManyQueryParams(r.URL.Query()["type"])
		transactions, err := service.GetTransactionsByAccount(
			r.Context(),
			accountID,
			txTypes,
		)
		if err != nil {
			return handlers.WrapError(
				err,
				"unable to get transactions for account",
				http.StatusBadRequest,
			)
		}
		return handlers.RenderContent(
			r.Context(),
			transactions,
			w,
			http.StatusOK,
		)
	})
}

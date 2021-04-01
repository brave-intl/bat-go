package eyeshade

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/go-chi/chi"
)

var (
	validTransactionTypes = map[string]bool{
		"contributions": true,
		"referrals":     true,
	}
)

// AccountsRouter has all information on account info
func (service *Service) AccountsRouter() chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/earnings/{type}/total", middleware.InstrumentHandler(
		"GetAccountEarningsTotal",
		service.GetAccountEarningsTotal(),
	))
	r.Method("GET", "/settlements/{type}/total", middleware.InstrumentHandler(
		"GetAccountSettlementEarningsTotal",
		service.GetAccountSettlementEarningsTotal(),
	))
	r.Method("GET", "/balances", middleware.InstrumentHandler(
		"AccountBalances",
		service.GetBalances(),
	))
	r.Method("GET", "/{account}/transactions", middleware.InstrumentHandler(
		"AccountTransactions",
		service.GetTransactions(),
	))
	return r
}

// GetAccountEarningsTotal retrieves the earnings of a limited number of accounts
func (service *Service) GetAccountEarningsTotal() handlers.AppHandler {
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

// GetAccountSettlementEarningsTotal retrieves the earnings of a limited number of accounts
func (service *Service) GetAccountSettlementEarningsTotal() handlers.AppHandler {
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

// GetBalances retrieves the balances for a given set of identifiers
func (service *Service) GetBalances() handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		accountIDs := r.URL.Query()["account"] // access map directly to get all items
		includePending := r.URL.Query().Get("pending") == "true"
		balances, err := service.Balances(r.Context(), accountIDs, includePending)
		if err != nil {
			return handlers.WrapError(err, "unable to get balances for account earnings", http.StatusBadRequest)
		}
		return handlers.RenderContent(r.Context(), balances, w, http.StatusOK)
	})
}

// GetTransactions retrieves the transactions for a given set of identifiers
func (service *Service) GetTransactions() handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		accountID := chi.URLParam(r, "account")
		txTypes := strings.Split(
			strings.Join(r.URL.Query()["type"], ","),
			",",
		)
		transactions, err := service.Transactions(
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

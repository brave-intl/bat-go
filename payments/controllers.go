package payments

import (
	"fmt"
	"net/http"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/go-chi/chi"
)

// PrepareHandler - handler to get current relative exchange rates
func PrepareHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			logger = logging.Logger(ctx, "PrepareHandler")
			req    = []*Transaction{}
		)

		// read the transactions in the body
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}
		// validate the list of transactions
		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		logger.Debug().Str("transactions", fmt.Sprintf("%+v", req)).Msg("handling prepare request")

		if err := service.InsertTransactions(ctx, req...); err != nil {
			return handlers.WrapError(err, "failed to insert transactions", http.StatusInternalServerError)
		}

		return nil
	})
}

// SubmitHandler - handler to perform submission of transactions to custodian
func SubmitHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			logger = logging.Logger(ctx, "SubmitHandler")
		)

		logger.Info().Msg("handling submit request")
		// FIXME - do the submission
		return nil
	})
}

// StatusHandler - handler to perform submission of transactions to custodian
func StatusHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			documentID = chi.URLParam(r, "documentID")
			logger     = logging.Logger(ctx, "StatusHandler")
		)

		logger.Info().Str("documentID", documentID).Msg("handling status request")
		// FIXME - do the status
		return nil
	})
}

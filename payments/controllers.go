package payments

import (
	"net/http"

	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/go-chi/chi"
)

// PrepareHandler - handler to get current relative exchange rates
func PrepareHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			custodian = chi.URLParam(r, "custodian")
		)

		// get logger from context
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			ctx, logger = logging.SetupLogger(ctx)
		}

		logger.Info().Str("custodian", custodian).Msg("handling prepare request")

		// FIXME - do the prepare

		return nil
	})
}

// SubmitHandler - handler to perform submission of transactions to custodian
func SubmitHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		// get logger from context
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			ctx, logger = logging.SetupLogger(ctx)
		}

		logger.Info().Msg("handling submit request")
		// FIXME - do the submission
		return nil
	})
}

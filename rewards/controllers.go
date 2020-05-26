package rewards

import (
	"errors"
	"net/http"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/logging"
)

// GetParametersHandler - handler to get reward parameters
func GetParametersHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		// response structure
		var parameters *Parameters

		// get logger from context
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			ctx, logger = logging.SetupLogger(ctx)
		}

		// ratios-service url
		rs, err := appctx.GetStringFromContext(ctx, appctx.RatiosServerCTXKey)
		if err != nil {
			// we are in a degraded state, as we do not have a ratios url
			logger.Error().Err(err).Msg("failed to get ratios server url from context")
			return handlers.WrapError(err, "degraded: no access to ratios", http.StatusInternalServerError)
		}

		// in here we need to validate our currency
		var currency = new(BaseCurrency)
		if err = inputs.DecodeAndValidate(ctx, currency, []byte(r.URL.Query().Get("currency"))); err != nil {
			if errors.Is(err, ErrBaseCurrencyEmpty) {
				*currency = BaseCurrency("USD")
			} else if errors.Is(err, ErrBaseCurrencyInvalid) {
				logger.Error().Err(err).Msg("invalid currency input from caller")
				return handlers.ValidationError(
					"Error validating currency url parameter",
					map[string]interface{}{
						"err":      err.Error(),
						"currency": "invalid currency",
					},
				)
			} else {
				// degraded, unknown error when validating/decoding
				logger.Error().Err(err).Msg("unforseen error in decode and validation")
				return handlers.WrapError(err, "degraded: ", http.StatusInternalServerError)
			}
		}

		logger.Debug().
			Str("ratios-service", rs).
			Str("currency", currency.String()).
			Msg("in GetParametersHandler")

		parameters, err = service.GetParameters(r.Context(), currency)
		if err != nil {
			logger.Error().Err(err).Msg("failed to get reward parameters")
			return handlers.WrapError(err, "failed to get parameters", http.StatusInternalServerError)
		}
		return handlers.RenderContent(r.Context(), parameters, w, http.StatusOK)
	})
}

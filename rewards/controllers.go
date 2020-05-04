package rewards

import (
	"net/http"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
)

// GetParametersHandler - handler to get reward parameters
func GetParametersHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		// response structure
		var parameters *Parameters

		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			ctx, logger = logging.SetupLogger(ctx)
		}

		// ratios-service url
		rs, _ := appctx.GetStringFromContext(ctx, appctx.RatiosServerCTXKey)
		rt, _ := appctx.GetStringFromContext(ctx, appctx.RatiosAccessTokenCTXKey)

		logger.Info().
			Str("ratios-service", rs).
			Str("ratios-token", rt).
			Msg("in GetParametersHandler")
		// in here we need to validate our currency

		parameters, err = service.GetParameters(r.Context())
		if err != nil {
			return handlers.WrapError(err, "Error create api keys", http.StatusInternalServerError)
		}
		return handlers.RenderContent(r.Context(), parameters, w, http.StatusOK)
	})
}

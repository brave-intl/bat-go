package payments

import (
	"fmt"
	"net/http"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/requestutils"
)

type configurationHandlerRequest map[appctx.CTXKey]interface{}

// ConfigurationHandler - handler to set the location of the current configuration
func ConfigurationHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			logger = logging.Logger(ctx, "ConfigurationHandler")
			req    = configurationHandlerRequest{}
		)

		// read the transactions in the body
		err := requestutils.ReadJSON(ctx, r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}
		logger.Info().Str("configurations", fmt.Sprintf("%+v", req)).Msg("handling configuration request")

		// set all the new configurations (will be picked up in future requests by configuration middleware
		service.baseCtx = appctx.MapToContext(service.baseCtx, req)

		// TODO: get the secrets file location
		if uri, ok := req[appctx.SecretsURICTXKey].(string); ok {
			logger.Info().Str("uri", uri).Msg("secrets location")
			// go get secrets, insert onto baseCtx
			secrets, err := service.secretMgr.RetrieveSecrets(ctx, uri)
			if err != nil {
				logger.Error().Err(err).Msg("error retrieving secrets")
				return handlers.WrapError(err, "error retrieving secrets", http.StatusInternalServerError)
			}
			service.baseCtx = appctx.MapToContext(service.baseCtx, secrets)
		}

		// return ok, at this point all new requests will use the new baseCtx of the service
		return handlers.RenderContent(r.Context(), nil, w, http.StatusOK)
	})
}

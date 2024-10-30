package rewards

import (
	"errors"
	"net/http"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/inputs"
	"github.com/brave-intl/bat-go/libs/logging"
)

func GetParametersHandler(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var currencyInput = r.URL.Query().Get("currency")
		if currencyInput == "" {
			currencyInput = "USD"
		}

		ctx := r.Context()

		lg := logging.Logger(ctx, "rewards").With().Str("func", "GetParametersHandler").Logger()

		currency := new(BaseCurrency)
		if err := inputs.DecodeAndValidate(ctx, currency, []byte(currencyInput)); err != nil {
			lg.Error().Err(err).Msg("failed decode and validate")

			if errors.Is(err, ErrBaseCurrencyInvalid) {
				return handlers.ValidationError("Error validating currency url parameter", map[string]interface{}{
					"err":      err.Error(),
					"currency": "invalid currency",
				})
			}

			return handlers.WrapError(err, "degraded: ", http.StatusInternalServerError)
		}

		parameters, err := service.GetParameters(ctx, currency)
		if err != nil {
			lg.Error().Err(err).Msg("failed to get reward parameters")

			return handlers.WrapError(err, "failed to get parameters", http.StatusInternalServerError)
		}

		return handlers.RenderContent(ctx, parameters, w, http.StatusOK)
	}
}

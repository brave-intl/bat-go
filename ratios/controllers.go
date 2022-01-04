package ratios

import (
	"errors"
	"fmt"
	"net/http"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/go-chi/chi"
)

// GetRelativeHandler - handler to get current relative exchange rates
func GetRelativeHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			coinIDsInput      = chi.URLParam(r, "coinIDs")
			vsCurrenciesInput = chi.URLParam(r, "vsCurrencies")
			durationInput     = chi.URLParam(r, "duration")
		)

		// get logger from context
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			ctx, logger = logging.SetupLogger(ctx)
		}

		var coinIDs = new(CoingeckoCoinList)
		fmt.Println(coinIDs)
		if err = inputs.DecodeAndValidate(ctx, coinIDs, []byte(coinIDsInput)); err != nil {
			if errors.Is(err, ErrCoingeckoCoinInvalid) {
				logger.Error().Err(err).Msg("invalid coin input from caller")
				return handlers.ValidationError(
					"Error validating coin url parameter",
					map[string]interface{}{
						"err":     err.Error(),
						"coinIDs": "invalid coin",
					},
				)
			}
			// degraded, unknown error when validating/decoding
			logger.Error().Err(err).Msg("unforseen error in decode and validation")
			return handlers.WrapError(err, "degraded: ", http.StatusInternalServerError)
		}

		var vsCurrencies = new(CoingeckoVsCurrencyList)
		if err = inputs.DecodeAndValidate(ctx, vsCurrencies, []byte(vsCurrenciesInput)); err != nil {
			if errors.Is(err, ErrCoingeckoVsCurrencyInvalid) {
				logger.Error().Err(err).Msg("invalid vs currency input from caller")
				return handlers.ValidationError(
					"Error validating vs currency url parameter",
					map[string]interface{}{
						"err":          err.Error(),
						"vScurrencies": "invalid vs currency",
					},
				)
			}
			// degraded, unknown error when validating/decoding
			logger.Error().Err(err).Msg("unforseen error in decode and validation")
			return handlers.WrapError(err, "degraded: ", http.StatusInternalServerError)
		}

		var duration = new(CoingeckoDuration)
		if err = inputs.DecodeAndValidate(ctx, duration, []byte(durationInput)); err != nil {
			if errors.Is(err, ErrCoingeckoDurationInvalid) {
				logger.Error().Err(err).Msg("invalid duration input from caller")
				return handlers.ValidationError(
					"Error validating duration url parameter",
					map[string]interface{}{
						"err":      err.Error(),
						"duration": "invalid duration",
					},
				)
			}
			// degraded, unknown error when validating/decoding
			logger.Error().Err(err).Msg("unforseen error in decode and validation")
			return handlers.WrapError(err, "degraded: ", http.StatusInternalServerError)
		}

		rates, err := service.GetRelative(ctx, *coinIDs, *vsCurrencies, *duration)
		if err != nil {
			logger.Error().Err(err).Msg("failed to get relative exchange rate")
			return handlers.WrapError(err, "failed to get relative exchange rate", http.StatusInternalServerError)
		}
		return handlers.RenderContent(ctx, rates, w, http.StatusOK)
	})
}

// GetHistoryHandler - handler to get historical exchange rates
func GetHistoryHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			coinIDInput     = chi.URLParam(r, "coinID")
			vsCurrencyInput = chi.URLParam(r, "vsCurrency")
			durationInput   = chi.URLParam(r, "duration")
		)

		// get logger from context
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			ctx, logger = logging.SetupLogger(ctx)
		}

		var coinID = new(CoingeckoCoin)
		if err = inputs.DecodeAndValidate(ctx, coinID, []byte(coinIDInput)); err != nil {
			if errors.Is(err, ErrCoingeckoCoinInvalid) {
				logger.Error().Err(err).Msg("invalid coin input from caller")
				return handlers.ValidationError(
					"Error validating coin url parameter",
					map[string]interface{}{
						"err":     err.Error(),
						"coinIDs": "invalid coin",
					},
				)
			}
			// degraded, unknown error when validating/decoding
			logger.Error().Err(err).Msg("unforseen error in decode and validation")
			return handlers.WrapError(err, "degraded: ", http.StatusInternalServerError)
		}

		var vsCurrency = new(CoingeckoVsCurrency)
		if err = inputs.DecodeAndValidate(ctx, vsCurrency, []byte(vsCurrencyInput)); err != nil {
			if errors.Is(err, ErrCoingeckoVsCurrencyInvalid) {
				logger.Error().Err(err).Msg("invalid vs currency input from caller")
				return handlers.ValidationError(
					"Error validating vs currency url parameter",
					map[string]interface{}{
						"err":          err.Error(),
						"vsCurrencies": "invalid vs currency",
					},
				)
			}
			// degraded, unknown error when validating/decoding
			logger.Error().Err(err).Msg("unforseen error in decode and validation")
			return handlers.WrapError(err, "degraded: ", http.StatusInternalServerError)
		}

		var duration = new(CoingeckoDuration)
		if err = inputs.DecodeAndValidate(ctx, duration, []byte(durationInput)); err != nil {
			if errors.Is(err, ErrCoingeckoDurationInvalid) {
				logger.Error().Err(err).Msg("invalid duration input from caller")
				return handlers.ValidationError(
					"Error validating duration url parameter",
					map[string]interface{}{
						"err":      err.Error(),
						"duration": "invalid duration",
					},
				)
			}
			// degraded, unknown error when validating/decoding
			logger.Error().Err(err).Msg("unforseen error in decode and validation")
			return handlers.WrapError(err, "degraded: ", http.StatusInternalServerError)
		}

		rates, err := service.GetHistory(ctx, *coinID, *vsCurrency, *duration)
		if err != nil {
			logger.Error().Err(err).Msg("failed to get historical exchange rate")
			return handlers.WrapError(err, "failed to get historical exchange rate", http.StatusInternalServerError)
		}
		return handlers.RenderContent(ctx, rates, w, http.StatusOK)
	})
}

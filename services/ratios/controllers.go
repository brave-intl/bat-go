package ratios

import (
	"errors"
	"net/http"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/inputs"
	"github.com/brave-intl/bat-go/libs/logging"
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
			err               error
		)

		// get logger from context
		logger := logging.Logger(ctx, "ratios.GetRelativeHandler")

		var coinIDs = new(CoingeckoCoinList)
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
			err             error
		)

		// get logger from context
		logger := logging.Logger(ctx, "ratios.GetHistoryHandler")

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

//MappingResponse - the response structure for the current mappings
type MappingResponse struct {
	IDToSymbol            map[string]string `json:"idToSymbol"`
	SymbolToID            map[string]string `json:"symbolToId"`
	ContractToID          map[string]string `json:"contractToId"`
	SupportedVsCurrencies map[string]bool   `json:"supportedVsCurrencies"`
}

// GetMappingHandler - handler to get current coin / currency mappings
func GetMappingHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		resp := MappingResponse{}

		resp.IDToSymbol = ctx.Value(appctx.CoingeckoIDToSymbolCTXKey).(map[string]string)
		resp.SymbolToID = ctx.Value(appctx.CoingeckoSymbolToIDCTXKey).(map[string]string)
		resp.ContractToID = ctx.Value(appctx.CoingeckoContractToIDCTXKey).(map[string]string)
		resp.SupportedVsCurrencies = ctx.Value(appctx.CoingeckoSupportedVsCurrenciesCTXKey).(map[string]bool)

		return handlers.RenderContent(ctx, resp, w, http.StatusOK)
	})
}

// GetCoinMarketsHandler - handler to get top currency data
func GetCoinMarketsHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			vsCurrencyInput = r.URL.Query().Get("vsCurrency")
			limitInput      = r.URL.Query().Get("limit")
			err             error
		)

		// get logger from context
		logger := logging.Logger(ctx, "ratios.GetCoinMarketsHandler")

		var vsCurrency = new(CoingeckoVsCurrency)
		if err = inputs.DecodeAndValidate(ctx, vsCurrency, []byte(vsCurrencyInput)); err != nil {
			if errors.Is(err, ErrCoingeckoVsCurrencyInvalid) {
				logger.Error().Err(err).Msg("invalid vs currency input from caller")
				return handlers.ValidationError(
					"Error validating vs currency url parameter",
					map[string]interface{}{
						"err":        err.Error(),
						"vsCurrency": "invalid vs currency",
					},
				)
			}
			// degraded, unknown error when validating/decoding
			logger.Error().Err(err).Msg("unforseen error in decode and validation")
			return handlers.WrapError(err, "degraded: ", http.StatusInternalServerError)
		}

		var limit = new(CoingeckoLimit)
		if err = inputs.DecodeAndValidate(ctx, limit, []byte(limitInput)); err != nil {
			if errors.Is(err, ErrCoingeckoLimitInvalid) {
				logger.Error().Err(err).Msg("invalid limit input from caller")
				return handlers.ValidationError(
					"Error validating vs currency url parameter",
					map[string]interface{}{
						"err":   err.Error(),
						"limit": "invalid limit",
					},
				)
			}
			logger.Error().Err(err).Msg("unforseen error in decode and validation")
			return handlers.WrapError(err, "degraded: ", http.StatusInternalServerError)
		}

		data, err := service.GetCoinMarkets(ctx, *vsCurrency, *limit)
		if err != nil {
			logger.Error().Err(err).Msg("failed to get top currencies")
			return handlers.WrapError(err, "failed to get top currencies", http.StatusInternalServerError)
		}
		return handlers.RenderContent(ctx, data, w, http.StatusOK)
	})
}

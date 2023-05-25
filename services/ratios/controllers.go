package ratios

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/brave-intl/bat-go/libs/clients/coingecko"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/inputs"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/requestutils"
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

			if errors.Is(err, ErrCoingeckoCoinEmpty) {
				logger.Error().Err(err).Msg("empty coin input from caller")
				return handlers.ValidationError(
					"Error validating coin url parameter",
					map[string]interface{}{
						"err":     err.Error(),
						"coinIDs": "empty coin",
					},
				)
			}

			if errors.Is(err, ErrCoingeckoCoinListLimit) {
				logger.Error().Err(err).Msg("coin list limit exceeded")
				return handlers.ValidationError(
					"Error validating coin url parameter",
					map[string]interface{}{
						"err":     err.Error(),
						"coinIDs": "coin list limit exceeded",
					},
				)
			}

			// degraded, unknown error when validating/decoding
			logger.Error().Err(err).Msg("unforseen error in decode and validation")
			return handlers.WrapError(err, "degraded: ", http.StatusBadRequest)
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

			if errors.Is(err, ErrCoingeckoVsCurrencyEmpty) {
				logger.Error().Err(err).Msg("empty vs currency input from caller")
				return handlers.ValidationError(
					"Error validating vs currency url parameter",
					map[string]interface{}{
						"err":          err.Error(),
						"vScurrencies": "empty vs currency",
					},
				)
			}

			if errors.Is(err, ErrCoingeckoVsCurrencyLimit) {
				logger.Error().Err(err).Msg("vs currency list limit exceeded")
				return handlers.ValidationError(
					"Error validating vs currency url parameter",
					map[string]interface{}{
						"err":          err.Error(),
						"vScurrencies": "vs currency list limit exceeded",
					},
				)
			}

			// degraded, unknown error when validating/decoding
			logger.Error().Err(err).Msg("unforseen error in decode and validation")
			return handlers.WrapError(err, "degraded: ", http.StatusBadRequest)
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
			return handlers.WrapError(err, "degraded: ", http.StatusBadRequest)
		}

		rates, err := service.GetRelative(ctx, *coinIDs, *vsCurrencies, *duration)
		if err != nil {
			logger.Error().Err(err).Msg("failed to get relative exchange rate")
			return handlers.WrapError(err, "failed to get relative exchange rate", http.StatusInternalServerError)
		}

		// Set Cache-Control header to match when the rates expire in the server cache,
		// and would be fetched from Coingecko again.
		maxAge := GetRelativeTTL*time.Second - time.Since(rates.LastUpdated)
		w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d", int(maxAge.Seconds())))
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
			return handlers.WrapError(err, "degraded: ", http.StatusBadRequest)
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

			if errors.Is(err, ErrCoingeckoVsCurrencyEmpty) {
				logger.Error().Err(err).Msg("empty vs currency input from caller")
				return handlers.ValidationError(
					"Error validating vs currency url parameter",
					map[string]interface{}{
						"err":          err.Error(),
						"vsCurrencies": "empty vs currency",
					},
				)
			}

			// degraded, unknown error when validating/decoding
			logger.Error().Err(err).Msg("unforseen error in decode and validation")
			return handlers.WrapError(err, "degraded: ", http.StatusBadRequest)
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
			return handlers.WrapError(err, "degraded: ", http.StatusBadRequest)
		}

		rates, err := service.GetHistory(ctx, *coinID, *vsCurrency, *duration)
		if err != nil {
			logger.Error().Err(err).Msg("failed to get historical exchange rate")
			return handlers.WrapError(err, "failed to get historical exchange rate", http.StatusInternalServerError)
		}

		w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d", duration.ToGetHistoryCacheDurationSeconds()))
		return handlers.RenderContent(ctx, rates, w, http.StatusOK)
	})
}

// MappingResponse - the response structure for the current mappings
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
			return handlers.WrapError(err, "degraded: ", http.StatusBadRequest)
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
			return handlers.WrapError(err, "degraded: ", http.StatusBadRequest)
		}

		data, err := service.GetCoinMarkets(ctx, *vsCurrency, *limit)
		if err != nil {
			logger.Error().Err(err).Msg("failed to get top currencies")
			return handlers.WrapError(err, "failed to get top currencies", http.StatusInternalServerError)
		}

		// Set Cache-Control header to match when the market data in the Reis cache expires,
		// and would be fetched from Coingecko again.
		maxAge := coingecko.CoinMarketsCacheTTLSeconds*time.Second - time.Since(data.LastUpdated)
		w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d", int(maxAge.Seconds())))
		return handlers.RenderContent(ctx, data, w, http.StatusOK)
	})
}

// StripeOnrampSessionRequest
type StripeOnrampSessionRequest struct {
	WalletAddress                string   `json:"wallet_address"`
	SourceCurrency               string   `json:"source_currency"`
	SourceExchangeAmount         string   `json:"source_exchange_amount"`
	DestinationNetwork           string   `json:"destination_network"`
	DestinationCurrency          string   `json:"destination_currency"`
	SupportedDestinationNetworks []string `json:"supported_destination_networks"`
}

// CreateStripeOnrampSessionResponse is an HTTP response that includes the Stripe onramp redirect URL
type CreateStripeOnrampSessionResponse struct {
	URL string `json:"url"`
}

func CreateStripeOnrampSessionsHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		logger := logging.Logger(ctx, "ratios.CreateStripeOnrampSessionsHandler")

		// Parse the payload
		var req StripeOnrampSessionRequest
		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		// Validate the request payload
		supportedDestinationNetworks := []string{"solana", "ethereum", "bitcoin", "polygon"}
		supportedDestinationCurrencies := []string{"eth", "matic", "sol", "usdc", "btc"}

		// Check if requested DestinationNetwork is in the supported list
		isValidNetwork := false
		for _, network := range supportedDestinationNetworks {
			if req.DestinationNetwork == network {
				isValidNetwork = true
				break
			}
		}
		if !isValidNetwork {
			return handlers.WrapError(
				fmt.Errorf("Invalid destination network: %s", req.DestinationNetwork),
				"Invalid destination network",
				http.StatusBadRequest,
			)
		}

		// Check if all SupportedDestinationNetworks in the request are in the supported list
		for _, requestedNetwork := range req.SupportedDestinationNetworks {
			isValidNetwork = false
			for _, network := range supportedDestinationNetworks {
				if requestedNetwork == network {
					isValidNetwork = true
					break
				}
			}
			if !isValidNetwork {
				return handlers.WrapError(
					fmt.Errorf("Unsupported network in SupportedDestinationNetworks: %s", requestedNetwork),
					"Unsupported network in SupportedDestinationNetworks",
					http.StatusBadRequest,
				)
			}
		}

		// Check if requested DestinationCurrency is in the supported list
		isValidCurrency := false
		for _, currency := range supportedDestinationCurrencies {
			if req.DestinationCurrency == currency {
				isValidCurrency = true
				break
			}
		}
		if !isValidCurrency {
			return handlers.WrapError(
				fmt.Errorf("Invalid destination currency: %s", req.DestinationCurrency),
				"Invalid destination currency",
				http.StatusBadRequest,
			)
		}

		// Create a session and retrieve a URL
		urlString, err := service.CreateStripeOnrampSessionsHandler(
			ctx,
			req.WalletAddress,
			req.SourceCurrency,
			req.SourceExchangeAmount,
			req.DestinationNetwork,
			req.DestinationCurrency,
			req.SupportedDestinationNetworks,
		)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create on ramp session")
			return handlers.WrapError(err, "Failed to create on ramp session", http.StatusInternalServerError)
		}

		response := CreateStripeOnrampSessionResponse{URL: urlString}
		return handlers.RenderContent(ctx, response, w, http.StatusOK)
	})
}

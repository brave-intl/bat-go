package ratios

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/clients/coingecko"
	ratiosclient "github.com/brave-intl/bat-go/libs/clients/ratios"
	"github.com/brave-intl/bat-go/libs/clients/stripe"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	srv "github.com/brave-intl/bat-go/libs/service"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

// NewService - create a new ratios service structure
func NewService(
	ctx context.Context,
	coingecko coingecko.Client,
	stripe stripe.Client,
	redis *redis.Client,
) *Service {
	return &Service{
		jobs:      []srv.Job{},
		coingecko: coingecko,
		stripe:    stripe,
		redis:     redis,
	}
}

// Service contains datastore
type Service struct {
	jobs []srv.Job
	// coingecko client
	coingecko coingecko.Client
	stripe    stripe.Client
	redis     *redis.Client
}

// Jobs - Implement srv.JobService interface
func (s *Service) Jobs() []srv.Job {
	return s.jobs
}

// InitService creates a service using the passed context
func InitService(ctx context.Context) (context.Context, *Service, error) {
	// get logger from context
	logger := logging.Logger(ctx, "ratios.InitService")

	redisAddr, err := appctx.GetStringFromContext(ctx, appctx.RatiosRedisAddrCTXKey)
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize the redis client")
		return ctx, nil, fmt.Errorf("failed to initialize redis client: %w", err)
	}

	opts, err := redis.ParseURL(redisAddr)
	if err != nil {
		logger.Error().Err(err).Msg("failed to parse redis URL")
		return ctx, nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}

	redis := redis.NewClient(opts)

	if err := redis.Ping(ctx).Err(); err != nil {
		logger.Error().Err(err).Msg("failed to initialize the redis client")
		return ctx, nil, fmt.Errorf("failed to initialize redis client: %w", err)
	}

	coingecko, err := coingecko.NewWithContext(ctx, redis)
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize the coingecko client")
		return ctx, nil, fmt.Errorf("failed to initialize coingecko client: %w", err)
	}

	stripe, err := stripe.NewWithContext(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize the stripe client")
		return ctx, nil, fmt.Errorf("failed to initialize stripe client: %w", err)
	}

	service := NewService(ctx, coingecko, stripe, redis)

	ctx, err = service.initializeCoingeckoCurrencies(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize the coingecko coin mappings")
		return ctx, nil, fmt.Errorf("failed to initialize coingecko coin mappings: %w", err)
	}

	service.jobs = []srv.Job{
		{
			Func:    service.RunNextRelativeCachePrepopulationJob,
			Cadence: 5 * time.Minute,
			Workers: 1,
		},
		{
			Func:    service.RemoveExpiredRelativeEntries,
			Cadence: 1 * time.Minute,
			Workers: 1,
		},
	}

	// Sigh, for compatibility with existing ratios mistakes
	decimal.MarshalJSONWithoutQuotes = true

	return ctx, service, nil
}

// RunNextRelativeCachePrepopulationJob takes the next job to prepopulate the relative cache and completes it
func (s *Service) RunNextRelativeCachePrepopulationJob(ctx context.Context) (bool, error) {
	topCoins, err := s.GetTopCoins(ctx, 500)
	if err != nil {
		return true, fmt.Errorf("failed to retrieve top coins: %w", err)
	}
	topCurrencies, err := s.GetTopCurrencies(ctx, 10)
	if err != nil {
		return true, fmt.Errorf("failed to retrieve top currencies: %w", err)
	}

	if len(topCoins) == 0 || len(topCurrencies) == 0 {
		return false, nil
	}

	rates, err := s.coingecko.FetchSimplePrice(ctx, topCoins.String(), topCurrencies.String(), true)
	if err != nil {
		return true, fmt.Errorf("failed to fetch price from coingecko: %w", err)
	}

	err = s.CacheRelative(ctx, *rates, true)
	if err != nil {
		return true, fmt.Errorf("failed to cache relative rates: %w", err)
	}

	return true, nil
}

// RemoveExpiredRelativeEntries removes all expired entries from the cache
// Workaround until Valkey implements HEXPIRE https://github.com/valkey-io/valkey/issues/640
func (s *Service) RemoveExpiredRelativeEntries(ctx context.Context) (bool, error) {
	logger := logging.Logger(ctx, "ratios.RemoveExpiredRelativeEntries")

	var cursor uint64 = 0
	batchSize := 500
	totalRemoved := 0

	for {
		// Get a batch of entries using HSCAN
		keys, nextCursor, err := s.redis.HScan(ctx, "relative", cursor, "", int64(batchSize)).Result()
		if err != nil {
			return false, fmt.Errorf("failed to scan relative cache: %w", err)
		}

		// Process entries in pairs (key, value)
		keysToDelete := []string{}
		for i := 0; i < len(keys); i += 2 {
			coin := keys[i]
			dataStr := keys[i+1]

			// Parse the data
			var data ratiosclient.RelativeResponse
			if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
				logger.Warn().Err(err).Str("coin", coin).Msg("failed to unmarshal relative cache entry, will remove")
				keysToDelete = append(keysToDelete, coin)
				continue
			}

			// Check if entry is stale based on GetRelativeTTL
			if time.Since(data.LastUpdated) > GetRelativeTTL*time.Second {
				keysToDelete = append(keysToDelete, coin)
			}
		}

		// Delete stale entries in a single operation if any found
		if len(keysToDelete) > 0 {
			if _, err := s.redis.HDel(ctx, "relative", keysToDelete...).Result(); err != nil {
				logger.Error().Err(err).Strs("coins", keysToDelete).Msg("failed to delete stale entries")
			} else {
				totalRemoved += len(keysToDelete)
			}
		}

		// Update cursor for next iteration
		cursor = nextCursor

		// Exit when we've scanned the entire hash
		if cursor == 0 {
			break
		}
	}

	if totalRemoved > 0 {
		logger.Info().Int("removed", totalRemoved).Msg("removed expired entries from relative cache")
	}

	return true, nil
}

// GetRelative - respond to caller with the relative exchange rates
func (s *Service) GetRelative(
	ctx context.Context,
	coinIDs CoingeckoCoinList,
	vsCurrencies CoingeckoVsCurrencyList,
	duration CoingeckoDuration,
) (*ratiosclient.RelativeResponse, error) {
	// get logger from context
	logger := logging.Logger(ctx, "ratios.GetRelative")

	// record coin / currency usage
	err := s.RecordCoinsAndCurrencies(ctx, []CoingeckoCoin(coinIDs), []CoingeckoVsCurrency(vsCurrencies))
	if err != nil {
		logger.Error().Err(err).Msg("failed to record coin / currency statistics")
	}

	// attempt to fetch from cache
	rates, updated, err := s.GetRelativeFromCache(ctx, vsCurrencies, []CoingeckoCoin(coinIDs)...)
	if err != nil || rates == nil {
		if err != nil {
			logger.Debug().Err(err).Msg("failed to fetch cached relative rates")
		}
		rates, err = s.coingecko.FetchSimplePrice(ctx, coinIDs.String(), vsCurrencies.String(), true)
		if err != nil {
			logger.Error().Err(err).Msg("failed to fetch price from coingecko")
			return nil, fmt.Errorf("failed to fetch price from coingecko: %w", err)
		}
		updated = time.Now()
	}

	if duration != "1d" {
		// fill change with 0s ( it's unused for multiple coinIDs and we will overwrite for single )
		out := map[string]map[string]decimal.Decimal{}
		for k, v := range *rates {
			innerOut := map[string]decimal.Decimal{}
			for kk, vv := range v {
				if !strings.HasSuffix(kk, "_24h_change") {
					innerOut[kk+"_timeframe_change"] = decimal.Zero
					innerOut[kk] = vv
				}
			}
			out[k] = innerOut
		}

		if len(coinIDs) == 1 {
			// request history for duration to calculate change
			chart, _, err := s.coingecko.FetchMarketChart(
				ctx,
				coinIDs[0].String(),
				vsCurrencies[0].String(),
				duration.ToDays(),
				duration.ToGetHistoryCacheDurationSeconds(),
			)
			if err != nil {
				logger.Error().Err(err).Msg("failed to fetch chart from coingecko")
				return nil, fmt.Errorf("failed to fetch chart from coingecko: %w", err)
			}

			current := out[coinIDs[0].String()][vsCurrencies[0].String()]
			previous := chart.Prices[0][1]
			change := decimal.Zero
			// division by error when previous is zero
			if !previous.IsZero() {
				change = current.Sub(previous).Div(previous).Mul(decimal.NewFromFloat(100))
			}

			out[coinIDs[0].String()][vsCurrencies[0].String()+"_timeframe_change"] = change
		}

		tmp := coingecko.SimplePriceResponse(out)
		rates = &tmp
	}

	return &ratiosclient.RelativeResponse{
		Payload:     mapSimplePriceResponse(ctx, *rates, duration, coinIDs, vsCurrencies),
		LastUpdated: updated,
	}, nil
}

// HistoryResponse - the response structure for history calls
type HistoryResponse struct {
	Payload     coingecko.MarketChartResponse `json:"payload"`
	LastUpdated time.Time                     `json:"lastUpdated"`
}

// GetHistory - respond to caller with historical exchange rates
func (s *Service) GetHistory(ctx context.Context, coinID CoingeckoCoin, vsCurrency CoingeckoVsCurrency, duration CoingeckoDuration) (*HistoryResponse, error) {
	// get logger from context
	logger := logging.Logger(ctx, "ratios.GetHistory")

	err := s.RecordCoinsAndCurrencies(ctx, []CoingeckoCoin{coinID}, []CoingeckoVsCurrency{vsCurrency})
	if err != nil {
		logger.Error().Err(err).Msg("failed to record coin / currency statistics")
	}

	chart, updated, err := s.coingecko.FetchMarketChart(
		ctx,
		coinID.String(),
		vsCurrency.String(),
		duration.ToDays(),
		duration.ToGetHistoryCacheDurationSeconds(),
	)
	if err != nil {
		logger.Error().Err(err).Msg("failed to fetch chart from coingecko")
		return nil, fmt.Errorf("failed to fetch chart from coingecko: %w", err)
	}

	return &HistoryResponse{
		Payload:     *chart,
		LastUpdated: updated,
	}, nil
}

// GetCoinMarketsResponse - the response structure for top currency calls
type GetCoinMarketsResponse struct {
	Payload     coingecko.CoinMarketResponse `json:"payload"`
	LastUpdated time.Time                    `json:"lastUpdated"`
}

// GetCoinMarkets - respond to caller with top currencies
func (s *Service) GetCoinMarkets(
	ctx context.Context,
	vsCurrency CoingeckoVsCurrency,
	limit CoingeckoLimit,
) (*GetCoinMarketsResponse, error) {

	// get logger from context
	logger := logging.Logger(ctx, "ratios.GetCoinMarkets")

	payload, updated, err := s.coingecko.FetchCoinMarkets(ctx, vsCurrency.String(), limit.Int())
	if err != nil {
		logger.Error().Err(err).Msg("failed to fetch coin markets data from coingecko")
		return nil, fmt.Errorf("failed to fetch coin markets data from coingecko: %w", err)
	}

	return &GetCoinMarketsResponse{
		Payload:     *payload,
		LastUpdated: updated,
	}, nil
}

// CreateStripeOnrampSessionsHandler - respond to caller with an onramp URL
func (s *Service) CreateStripeOnrampSessionsHandler(
	ctx context.Context,
	walletAddress string,
	sourceCurrency string,
	sourceExchangeAmount string,
	destinationNetwork string,
	destinationCurrency string,
	supportedDestinationNetworks []string,
) (string, error) {
	logger := logging.Logger(ctx, "ratios.CreateStripeOnrampSessionsHandler")
	payload, err := s.stripe.CreateOnrampSession(
		ctx,
		"redirect",
		walletAddress,
		sourceCurrency,
		sourceExchangeAmount,
		destinationNetwork,
		destinationCurrency,
		supportedDestinationNetworks,
	)

	if err != nil {
		logger.Error().Err(err).Msg("failed to create onramp session with stripe")
		return "", fmt.Errorf("error creating onramp session with stripe: %w", err)
	}

	return payload.RedirectURL, nil
}

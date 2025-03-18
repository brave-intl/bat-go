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
	logger.Debug().Msg("Starting relative cache cleanup job")

	// Get all coins from the tracking set instead of using KEYS
	coinMembers, err := s.redis.SMembers(ctx, "relative_coins").Result()
	if err != nil {
		logger.Error().Err(err).Msg("Failed to get coin members from set")
		return false, fmt.Errorf("failed to get coin members from set: %w", err)
	}

	logger.Debug().Int("coinCount", len(coinMembers)).Msg("Found coins to check")
	totalRemoved := 0
	coinsRemoved := 0
	scannedCount := 0
	coinsToRemoveFromSet := []string{}

	// Use a pipeline for batch operations
	pipe := s.redis.Pipeline()

	// For each coin in the set, check its currency entries
	for _, coin := range coinMembers {
		coinKey := fmt.Sprintf("relative:%s", coin)

		// Get all currency entries for this coin
		currencyData, err := s.redis.HGetAll(ctx, coinKey).Result()
		if err != nil {
			logger.Error().Err(err).Str("coin", coin).Msg("Failed to get currency data")
			continue
		}

		// If coin hash doesn't exist or is empty, mark for removal from set
		if len(currencyData) == 0 {
			logger.Debug().Str("coin", coin).Msg("Coin hash is empty, will remove from tracking set")
			coinsToRemoveFromSet = append(coinsToRemoveFromSet, coin)
			continue
		}

		scannedCount += len(currencyData)
		logger.Debug().Str("coin", coin).Int("currencies", len(currencyData)).Msg("Retrieved currency entries")

		// Check if all currency entries are stale
		staleKeys := []string{}
		allStale := true
		validCount := 0
		staleCount := 0

		for currKey, dataStr := range currencyData {
			var currData CurrencyData
			if err := json.Unmarshal([]byte(dataStr), &currData); err != nil {
				logger.Warn().Err(err).Str("coin", coin).Str("currency", currKey).Msg("Failed to unmarshal currency data, will remove")
				staleKeys = append(staleKeys, currKey)
				staleCount++
				continue
			}

			// Check if the currency entry is stale
			if time.Since(currData.LastUpdated) > GetRelativeTTL*time.Second {
				logger.Debug().Str("coin", coin).Str("currency", currKey).Time("lastUpdated", currData.LastUpdated).Msg("Stale currency entry found")
				staleKeys = append(staleKeys, currKey)
				staleCount++
			} else {
				validCount++
				allStale = false
			}
		}

		logger.Debug().Str("coin", coin).Int("validCurrencies", validCount).Int("staleCurrencies", staleCount).Bool("allStale", allStale).Msg("Checked entry currencies")

		// If we have stale currency entries
		if len(staleKeys) > 0 {
			// If all entries are stale, delete the entire hash and remove from tracking set
			if allStale && len(currencyData) > 0 {
				logger.Debug().Str("coin", coin).Msg("All currencies are stale, deleting entire coin hash")
				pipe.Del(ctx, coinKey)
				coinsToRemoveFromSet = append(coinsToRemoveFromSet, coin)
				totalRemoved += len(currencyData)
				coinsRemoved++
			} else {
				// Otherwise, just delete the stale currency entries
				logger.Debug().Str("coin", coin).Strs("staleCurrencies", staleKeys).Msg("Deleting stale currency entries")
				pipe.HDel(ctx, coinKey, staleKeys...)
				totalRemoved += len(staleKeys)
			}
		} else {
			logger.Debug().Str("coin", coin).Msg("No stale entries found for this coin")
		}
	}

	// Remove coins from the tracking set if needed
	if len(coinsToRemoveFromSet) > 0 {
		strs := make([]interface{}, len(coinsToRemoveFromSet))
		for i, v := range coinsToRemoveFromSet {
			strs[i] = v
		}
		pipe.SRem(ctx, "relative_coins", strs...)
	}

	// Execute all operations in a transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to execute pipeline")
		return false, fmt.Errorf("failed to execute pipeline: %w", err)
	}

	if totalRemoved > 0 {
		logger.Info().Int("entriesRemoved", totalRemoved).Int("coinsRemoved", coinsRemoved).Int("scanned", scannedCount).Msg("Removed expired entries from relative cache")
	} else {
		logger.Info().Int("scanned", scannedCount).Msg("No expired entries found in relative cache")
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

	if len(coinIDs) == 0 {
		logger.Warn().Msg("coinIDs is empty, returning empty payload")
		return &ratiosclient.RelativeResponse{
			Payload:     map[string]map[string]decimal.Decimal{},
			LastUpdated: time.Now(),
		}, nil
	}

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

		// insert into cache
		if err := s.CacheRelative(ctx, *rates, false); err != nil {
			logger.Error().Err(err).Msg("failed to cache relative rates")
		}
		updated = time.Now()
	}

	// Transform rates to copy 24h change values to timeframe change
	out := map[string]map[string]decimal.Decimal{}
	for k, v := range *rates {
		innerOut := map[string]decimal.Decimal{}
		for kk, vv := range v {
			if strings.HasSuffix(kk, "_24h_change") {
				// Copy 24h change to timeframe change
				innerOut[strings.TrimSuffix(kk, "_24h_change")+"_timeframe_change"] = vv
			}
			innerOut[kk] = vv
		}
		out[k] = innerOut
	}

	tmp := coingecko.SimplePriceResponse(out)
	rates = &tmp

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

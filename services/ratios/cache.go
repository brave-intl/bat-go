package ratios

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/clients/coingecko"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

const (
	// The amount of seconds price data can be in the Redis cache
	// before it is considered stale
	GetRelativeTTL = 900
	// The maximum number of entries that can be in the relative cache (excluding the top coins)
	MaxRelativeEntries = 10000
)

// CurrencyData represents price data for a single currency
type CurrencyData struct {
	Price       decimal.Decimal `json:"price"`
	Change24h   decimal.Decimal `json:"24h_change"`
	LastUpdated time.Time       `json:"last_updated"`
}

// GetTopCoins - get the top coins
func (s *Service) GetTopCoins(ctx context.Context, limit int) (CoingeckoCoinList, error) {
	var resp CoingeckoCoinList
	coinCacheKey := fmt.Sprintf("coins-%s", time.Now().Format("2006-01-02"))

	tmp, err := s.redis.ZRevRangeByScore(ctx, coinCacheKey, &redis.ZRangeBy{
		Min:    "0",
		Max:    "+inf",
		Offset: 0,
		Count:  int64(limit),
	}).Result()
	if err != nil {
		return resp, err
	}

	list := make([]CoingeckoCoin, len(tmp))
	for i, coin := range tmp {
		list[i] = CoingeckoCoin{coin: coin}
	}

	resp = CoingeckoCoinList(list)
	return resp, nil
}

// GetTopCurrencies - get the top currencies
func (s *Service) GetTopCurrencies(ctx context.Context, limit int) (CoingeckoVsCurrencyList, error) {
	var resp CoingeckoVsCurrencyList
	currencyCacheKey := fmt.Sprintf("currencies-%s", time.Now().Format("2006-01-02"))

	tmp, err := s.redis.ZRevRangeByScore(ctx, currencyCacheKey, &redis.ZRangeBy{
		Min:    "0",
		Max:    "+inf",
		Offset: 0,
		Count:  int64(limit),
	}).Result()
	if err != nil {
		return resp, err
	}

	list := make([]CoingeckoVsCurrency, len(tmp))
	for i, currency := range tmp {
		list[i] = CoingeckoVsCurrency(currency)
	}

	resp = CoingeckoVsCurrencyList(list)
	return resp, nil
}

// RecordCoinsAndCurrencies - record the coins and currencies in the cache
func (s *Service) RecordCoinsAndCurrencies(ctx context.Context, coinIds []CoingeckoCoin, vsCurrencies []CoingeckoVsCurrency) error {
	coinCacheKey := fmt.Sprintf("coins-%s", time.Now().Format("2006-01-02"))
	currencyCacheKey := fmt.Sprintf("currencies-%s", time.Now().Format("2006-01-02"))

	pipe := s.redis.Pipeline()

	for _, coin := range coinIds {
		pipe.ZIncrBy(ctx, coinCacheKey, 1, coin.String())
	}

	for _, currency := range vsCurrencies {
		pipe.ZIncrBy(ctx, currencyCacheKey, 1, currency.String())
	}

	_, err := pipe.Exec(ctx)
	return err
}

// CacheRelative - cache the relative values
func (s *Service) CacheRelative(ctx context.Context, resp coingecko.SimplePriceResponse, ignoreLimit bool) error {
	// Check if we're at the entry limit, unless ignoreLimit is true
	if !ignoreLimit {
		// Get current count of coins in the set
		entriesCount, err := s.redis.SCard(ctx, "relative_coins").Result()
		if err != nil && err != redis.Nil {
			return fmt.Errorf("failed to get relative cache size: %w", err)
		}

		// Check if adding these new entries would exceed the maximum limit
		if entriesCount+int64(len(resp)) > int64(MaxRelativeEntries) {
			return fmt.Errorf("relative cache would exceed maximum entries limit (%d) by adding %d entries",
				MaxRelativeEntries, len(resp))
		}
	}

	now := time.Now()
	pipe := s.redis.Pipeline()

	// Track all coins to add to our set
	coinsToAdd := make([]interface{}, 0, len(resp))

	for coin, rates := range resp {
		coinKey := fmt.Sprintf("relative:%s", coin)
		data := make(map[string]interface{})

		for currKey, value := range rates {
			if strings.HasSuffix(currKey, "_24h_change") {
				continue // Skip, as we'll handle these together with their price
			}

			// Extract currency code from key
			currencyCode := currKey

			// Create or update the currency data
			currData := CurrencyData{
				Price:       value,
				Change24h:   decimal.Zero, // Default to zero, will update if available
				LastUpdated: now,
			}

			// Check if there's a corresponding change value
			if changeValue, ok := rates[currKey+"_24h_change"]; ok {
				currData.Change24h = changeValue
			}

			// Serialize the currency data
			bytes, err := json.Marshal(&currData)
			if err != nil {
				return err
			}

			// Add to the hash map for this coin
			data[currencyCode] = string(bytes)
		}

		// Set the hash for this coin
		pipe.HSet(ctx, coinKey, data)

		// Add coin to our tracking set
		coinsToAdd = append(coinsToAdd, coin)
	}

	// Add all coins to the relative_coins set
	if len(coinsToAdd) > 0 {
		pipe.SAdd(ctx, "relative_coins", coinsToAdd...)
	}

	// Execute all operations in a transaction
	_, err := pipe.Exec(ctx)
	return err
}

// GetRelativeFromCache - get the relative response from the cache
func (s *Service) GetRelativeFromCache(ctx context.Context, vsCurrencies CoingeckoVsCurrencyList, coinIds ...CoingeckoCoin) (*coingecko.SimplePriceResponse, time.Time, error) {
	// Initialize with current time, will be updated to oldest timestamp from cache
	updated := time.Now()

	// Track entries with all requested currencies
	resp := make(map[string]map[string]decimal.Decimal, len(coinIds))
	var missingData []string

	// Create a pipeline for efficient retrieval
	pipe := s.redis.Pipeline()
	cmds := make(map[string]*redis.SliceCmd, len(coinIds))

	// Build list of currencies to fetch for each coin
	currencyKeys := make([]string, len(vsCurrencies))
	for i, curr := range vsCurrencies {
		currencyKeys[i] = curr.String()
	}

	// Queue up the HMGet commands for each coin
	for _, coin := range coinIds {
		coinStr := coin.String()
		coinKey := fmt.Sprintf("relative:%s", coinStr)
		cmd := pipe.HMGet(ctx, coinKey, currencyKeys...)
		cmds[coinStr] = cmd
	}

	// Execute the pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, updated, err
	}

	// Process the results
	for _, coin := range coinIds {
		coinStr := coin.String()
		cmd := cmds[coinStr]
		currencyValues, err := cmd.Result()

		if err != nil {
			missingData = append(missingData, fmt.Sprintf("error fetching data for coin: %s: %v", coinStr, err))
			continue
		}

		// Process the currency values for this coin
		coinRates := make(map[string]decimal.Decimal)
		oldestUpdate := updated
		hasAllCurrencies := true

		for i, currValue := range currencyValues {
			currencyStr := currencyKeys[i]

			// Missing data for this currency
			if currValue == nil {
				missingData = append(missingData, fmt.Sprintf("missing vs currency: %s for coin: %s", currencyStr, coinStr))
				hasAllCurrencies = false
				break
			}

			// Parse the currency data
			currValueStr, ok := currValue.(string)
			if !ok {
				missingData = append(missingData, fmt.Sprintf("invalid type for currency %s, expected string for coin: %s", currencyStr, coinStr))
				hasAllCurrencies = false
				break
			}

			var currData CurrencyData
			if err := json.Unmarshal([]byte(currValueStr), &currData); err != nil {
				missingData = append(missingData, fmt.Sprintf("error unmarshaling data for currency %s, coin %s: %v", currencyStr, coinStr, err))
				hasAllCurrencies = false
				break
			}

			// Check if this entry is stale
			if time.Since(currData.LastUpdated) > GetRelativeTTL*time.Second {
				// Data is stale, mark for refresh
				missingData = append(missingData, fmt.Sprintf("stale data for vs currency: %s for coin: %s", currencyStr, coinStr))
				hasAllCurrencies = false
				break
			}

			// Track the oldest update time
			if currData.LastUpdated.Before(oldestUpdate) {
				oldestUpdate = currData.LastUpdated
			}

			// Convert back to the original response format
			coinRates[currencyStr] = currData.Price
			coinRates[currencyStr+"_24h_change"] = currData.Change24h
		}

		// Only add complete coins to the response
		if hasAllCurrencies {
			// Update the oldest timestamp
			if oldestUpdate.Before(updated) {
				updated = oldestUpdate
			}

			resp[coinStr] = coinRates
		}
	}

	// If we have any missing or stale data, return error
	if len(missingData) > 0 {
		return nil, updated, fmt.Errorf("incomplete cache data: %s", strings.Join(missingData, "; "))
	}

	sResp := coingecko.SimplePriceResponse(resp)
	return &sResp, updated, nil
}

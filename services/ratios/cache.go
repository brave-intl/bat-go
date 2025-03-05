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

// CoinCacheData represents the structure of coin data stored in Redis
type CoinCacheData map[string]CurrencyData

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
		entriesCount, err := s.redis.HLen(ctx, "relative").Result()
		if err != nil {
			return fmt.Errorf("failed to get relative cache size: %w", err)
		}

		// Check if adding these new entries would exceed the maximum limit
		if entriesCount+int64(len(resp)) > int64(MaxRelativeEntries) {
			return fmt.Errorf("relative cache would exceed maximum entries limit (%d) by adding %d entries",
				MaxRelativeEntries, len(resp))
		}
	}

	now := time.Now()
	data := make(map[string]interface{})

	for coin, rates := range resp {
		coinData := CoinCacheData{}

		for currKey, value := range rates {
			if strings.HasSuffix(currKey, "_24h_change") {
				continue // Skip, as we'll handle these together with their price
			}

			// Extract currency code from key (e.g. "usd" from "usd", "usd_24h_change" is skipped above)
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

			coinData[currencyCode] = currData
		}

		bytes, err := json.Marshal(&coinData)
		if err != nil {
			return err
		}

		data[coin] = string(bytes)
	}

	return s.redis.HSet(ctx, "relative", data).Err()
}

// GetRelativeFromCache - get the relative response from the cache
func (s *Service) GetRelativeFromCache(ctx context.Context, vsCurrencies CoingeckoVsCurrencyList, coinIds ...CoingeckoCoin) (*coingecko.SimplePriceResponse, time.Time, error) {
	// Initialize with current time, will be updated to oldest timestamp from cache
	updated := time.Now()

	keys := make([]string, len(coinIds))
	for i, coin := range coinIds {
		keys[i] = coin.String()
	}

	rates, err := s.redis.HMGet(ctx, "relative", keys...).Result()
	if err != nil {
		return nil, updated, err
	}

	// Track entries with all requested currencies
	resp := make(map[string]map[string]decimal.Decimal, len(rates))

	for i, rate := range rates {
		coin := coinIds[i].String()

		if rate == nil {
			// Missing data for this coin
			return nil, updated, fmt.Errorf("missing rates for coin: %s", coin)
		}

		rateStr, ok := rate.(string)
		if !ok {
			return nil, updated, fmt.Errorf("invalid type for rate, expected string for coin: %s", coin)
		}

		var coinData CoinCacheData
		if err := json.Unmarshal([]byte(rateStr), &coinData); err != nil {
			return nil, updated, err
		}

		coinRates := make(map[string]decimal.Decimal)
		oldestUpdate := updated

		// Ensure all currencies exist for this coin
		for _, expectedCurrency := range vsCurrencies {
			currencyStr := expectedCurrency.String()
			currData, found := coinData[currencyStr]

			// If currency not found, indicate this coin needs to be fetched from API
			if !found {
				return nil, updated, fmt.Errorf("missing vs currency: %s for coin: %s", expectedCurrency, coin)
			}

			// Check if this entry is stale
			if time.Since(currData.LastUpdated) > GetRelativeTTL*time.Second {
				// Data is stale, but we'll still consider it valid for now
				// We track this by reducing the "updated" time which will be tested later
			}

			// Track the oldest update time
			if currData.LastUpdated.Before(oldestUpdate) {
				oldestUpdate = currData.LastUpdated
			}

			// Convert back to the original response format
			coinRates[currencyStr] = currData.Price
			coinRates[currencyStr+"_24h_change"] = currData.Change24h
		}

		// Update the oldest timestamp
		if oldestUpdate.Before(updated) {
			updated = oldestUpdate
		}

		resp[coin] = coinRates
	}

	// Check if any of the data is too old
	if time.Since(updated) > GetRelativeTTL*time.Second {
		return nil, updated, fmt.Errorf("cached rate is too old: %s", updated)
	}

	sResp := coingecko.SimplePriceResponse(resp)
	return &sResp, updated, nil
}

package ratios

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/libs/clients/coingecko"
	ratiosclient "github.com/brave-intl/bat-go/libs/clients/ratios"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

const (
	// The amount of seconds price data can be in the Redis cache
	// before it is considered stale
	GetRelativeTTL = 900
)

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
func (s *Service) CacheRelative(ctx context.Context, resp coingecko.SimplePriceResponse) error {
	now := time.Now()

	data := make(map[string]interface{})

	for coin, rates := range resp {
		var subResp ratiosclient.RelativeResponse
		payload := make(map[string]map[string]decimal.Decimal, 1)
		payload[coin] = rates
		subResp.Payload = payload
		subResp.LastUpdated = now

		bytes, err := json.Marshal(&subResp)
		if err != nil {
			return err
		}

		data[coin] = string(bytes)
	}

	return s.redis.HSet(ctx, "relative", data).Err()
}

// GetRelativeFromCache - get the relative response from the cache
func (s *Service) GetRelativeFromCache(ctx context.Context, vsCurrencies CoingeckoVsCurrencyList, coinIds ...CoingeckoCoin) (*coingecko.SimplePriceResponse, time.Time, error) {
	updated := time.Now()

	keys := make([]string, len(coinIds))
	for i, coin := range coinIds {
		keys[i] = coin.String()
	}

	rates, err := s.redis.HMGet(ctx, "relative", keys...).Result()
	if err != nil {
		return nil, updated, err
	}

	resp := make(map[string]map[string]decimal.Decimal, len(rates))
	for i, rate := range rates {
		coin := coinIds[i].String()

		if rate != nil {
			rateStr, ok := rate.(string)
			if !ok {
				return nil, updated, fmt.Errorf("invalid type for rate, expected string for coin: %s", coin)
			}

			var r ratiosclient.RelativeResponse
			if err := json.Unmarshal([]byte(rateStr), &r); err != nil {
				return nil, updated, err
			}
			// the least recently updated
			if r.LastUpdated.Before(updated) {
				updated = r.LastUpdated
				if time.Since(updated) > GetRelativeTTL*time.Second {
					return nil, updated, fmt.Errorf("cached rate is too old: %s", updated)
				}
			}

			// check that all vs currencies are included
			coinRate := r.Payload[coin]
			for _, expectedCurrency := range vsCurrencies {
				found := false
				for includedCurrency := range coinRate {
					if expectedCurrency.String() == includedCurrency {
						found = true
					}
				}
				if !found {
					return nil, updated, fmt.Errorf("missing vs currency: %s", expectedCurrency)
				}
			}
			resp[coin] = coinRate
		} else {
			return nil, updated, fmt.Errorf("missing rates for coin: %s", coin)
		}
	}

	sResp := coingecko.SimplePriceResponse(resp)
	return &sResp, updated, nil
}

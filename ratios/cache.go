package ratios

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/utils/clients/coingecko"
	"github.com/gomodule/redigo/redis"
	"github.com/shopspring/decimal"
)

func (s *Service) GetTopCoins(ctx context.Context, limit int) (CoingeckoCoinList, error) {
	conn := s.redis.Get()
	defer conn.Close()

	var resp CoingeckoCoinList
	coinCacheKey := fmt.Sprintf("coins-%s", time.Now().Format("2006-01-02"))

	tmp, err := redis.Strings(conn.Do("ZREVRANGEBYSCORE", coinCacheKey, "+inf", "0", "LIMIT", "0", limit))
	if err != nil {
		return resp, err
	}

	list := make([]CoingeckoCoin, len(tmp))
	for i, coin := range tmp {
		list[i] = CoingeckoCoin(coin)
	}

	resp = CoingeckoCoinList(list)
	return resp, nil
}

func (s *Service) GetTopCurrencies(ctx context.Context, limit int) (CoingeckoVsCurrencyList, error) {
	conn := s.redis.Get()
	defer conn.Close()

	var resp CoingeckoVsCurrencyList
	currencyCacheKey := fmt.Sprintf("currencies-%s", time.Now().Format("2006-01-02"))

	tmp, err := redis.Strings(conn.Do("ZREVRANGEBYSCORE", currencyCacheKey, "+inf", "0", "LIMIT", "0", limit))
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
func (s *Service) RecordCoinsAndCurrencies(ctx context.Context, coinIds []CoingeckoCoin, vsCurrencies []CoingeckoVsCurrency) error {
	conn := s.redis.Get()
	defer conn.Close()

	coinCacheKey := fmt.Sprintf("coins-%s", time.Now().Format("2006-01-02"))
	currencyCacheKey := fmt.Sprintf("currencies-%s", time.Now().Format("2006-01-02"))

	for _, coin := range coinIds {
		err := conn.Send("ZINCRBY", coinCacheKey, "1", coin.String())
		if err != nil {
			return err
		}
	}

	for _, currency := range vsCurrencies {
		err := conn.Send("ZINCRBY", currencyCacheKey, "1", currency.String())
		if err != nil {
			return err
		}
	}

	err := conn.Flush()
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) CacheRelative(ctx context.Context, resp coingecko.SimplePriceResponse) error {
	conn := s.redis.Get()
	defer conn.Close()

	now := time.Now()

	tmp := make([]interface{}, 1, (2*len(resp))+1)
	tmp[0] = "relative"

	for coin, rates := range resp {
		var subResp RelativeResponse
		payload := make(map[string]map[string]decimal.Decimal, 1)
		payload[coin] = rates
		subResp.Payload = payload
		subResp.LastUpdated = now

		bytes, err := json.Marshal(&subResp)
		if err != nil {
			return err
		}

		tmp = append(tmp, coin)
		tmp = append(tmp, bytes)
	}

	_, err := conn.Do("HMSET", tmp...)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) GetRelativeFromCache(ctx context.Context, vsCurrencies CoingeckoVsCurrencyList, coinIds ...CoingeckoCoin) (*coingecko.SimplePriceResponse, time.Time, error) {
	conn := s.redis.Get()
	defer conn.Close()

	updated := time.Now()

	tmp := make([]interface{}, 1, len(coinIds)+1)
	tmp[0] = "relative"
	for _, coin := range coinIds {
		tmp = append(tmp, coin.String())
	}
	rates, err := redis.Strings(conn.Do("HMGET", tmp...))
	if err != nil {
		return nil, updated, err
	}

	resp := make(map[string]map[string]decimal.Decimal, len(rates))
	for i, rate := range rates {
		coin := coinIds[i].String()

		if len(rate) > 0 {
			var r RelativeResponse
			err = json.Unmarshal([]byte(rate), &r)
			if err != nil {
				return nil, updated, err
			}
			// the least recently updated
			if r.LastUpdated.Before(updated) {
				updated = r.LastUpdated
				// FIXME check if response is too old
			}

			// check that all vs currencies are included
			coinRate := r.Payload[coin]
			for _, expectedCurrency := range vsCurrencies {
				found := false
				for includedCurrency, _ := range coinRate {
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

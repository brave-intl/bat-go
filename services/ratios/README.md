# Ratios

## GetRelative 

### Summary
Path: `GET /v2/relative/provider/coingecko/{coinIDs}/{vsCurrencies}/{duration}`
Params:
* `coinIDs` - Comma separated list of symbol, contract address, or coingecko ID of a coin to get price data for.  E.g bat,eth,cel,burp/jh
* `vsCurrencies` - The currencies to get price data in `usd,eth/` (much fewer options are suported here)
* `duration` - The timespan for the 'change in price' value that is included in the response

This endpoint fetches prices the rates of the supplied coinIDs against the vsCurrencies supplied. This data is sourced from Coingecko's `/simple/price?ids={ids}&vs_currencies={vs_currencies}` endpoint.  This Coingecko endpoint also returns a the 24 hour change in price, but not other timeframes.

The endpoint also includes the change in price over the course of of the duration supplied. This data is sourced from Coingecko's `/coins/{id}/market_chart` endpoint.


Example response:
```
/v2/relative/provider/coingecko/bat,chainlink/btc,usd/1w
{
 "payload": {
   "chainlink": {
     "btc": 0.00063075,
     "usd": 29.17,
     "btc_timeframe_change": -0.9999742658279261,
     "usd_timeframe_change": 0.1901162098990581
   },
   "bat": {
     "btc": 1.715e-05,
     "usd": 0.793188,
     "btc_timeframe_change": -0.9999993002916352,
     "usd_timeframe_change": -0.9676384677306338
   }
 },
 "lastUpdated": "2021-08-16T15:45:11.901Z"
}
```

### Caching
This endpoint uses Redis to cache parts of the response.

There is also potentially some mystery caching happening I cannot track down. You can reproduce by sending two identical requests of this endpoint in quick succession. You will request logs for the first, but not the second. Both requests will return a valid response. I don't believe the Redis cache is in use, because we would still see the second request logged - because request logging is handled by middleware that runs prior to fetching the data from redis. It could be CloudFront, but I don't see any deployments.

#### Data structures stored in cache
We store price data and price change data separately in Redis:

**Price data**
We cache the entire payload received from the Coingecko price endpoint `/simple/price`, with an additional `lastUpdated` timestamp representing when our app last fetched this data from Coingecko. The data is stored as a hash in Redis under the key "relative". Within the "relative" hash, data is indexed by coingecko ID, e.g. "basic-attention-token".
```
127.0.0.1:6379> HMGET "relative" "basic-attention-token"
1) "{\"payload\":{\"basic-attention-token\":{\"eth\":0.00020012,\"eth_24h_change\":-2.6022589094564683,\"usd\":0.327208,\"usd_24h_change\":1.815019912452554}},\"lastUpdated\":\"2022-09-08T17:47:36.813120619Z\"}"
```

**Market chart**
We cache the entire payload received from the Coingecko market chart endpoint `/coins/{id}/market_chart?days={days}`.

The cache key is the full coingecko URL. Note: we are also passing an unnecessary `&id={id}` query param in the endpoint.  This could be interferring with cache keys if cache entries are updated in two places, and one of the places does not add this param.
```
127.0.0.1:6379> GET "https://pro-api.coingecko.com/api/v3/coins/lido-dao-wormhole/market_chart?days=0.041666668&id=lido-dao-wormhole&vs_currency=usd"
"{\"payload\":\"{\\\"prices\\\":[[1662656777928,1.8860000771121217],[1662657088158,1.8771867383535248],[1662657321969,1.881620254331244],[1662657695341,1.8854145493207017],[1662658036215,1.8472653342186727],[1662658215650,1.881888541255327],[1662658637761,1.846343282069057],[1662658937581,1.8834030081812585],[1662659180815,1.8495881788292103],[1662659542077,1.8467497189116324],[1662659833619,1.8439360985080828],[1662660087053,1.8411601309603176],[1662660404000,1.8397213128904053]],\\\"market_caps\\\":[[1662656777928,0],[1662657088158,0],[1662657321969,0],[1662657695341,0],[1662658036215,0],[1662658215650,0],[1662658637761,0],[1662658937581,0],[1662659180815,0],[1662659542077,0],[1662659833619,0],[1662660087053,0],[1662660404000,0]],\\\"total_volumes\\\":[[1662656777928,41749.03809468673],[1662657088158,41251.090172596516],[1662657321969,41348.51114711865],[1662657695341,41243.5831174905],[1662658036215,49604.74873516559],[1662658215650,41165.07582420984],[1662658637761,49572.344926085316],[1662658937581,40964.27010973216],[1662659180815,49429.99463875508],[1662659542077,49233.952769597534],[1662659833619,49139.975121632255],[1662660087053,48928.73238191024],[1662660404000,48886.84101288481]]}\",\"lastUpdated\":\"2022-09-08T18:07:08.420028954Z\"}"
```

#### Endpoint logic
This is flow of a `GET /v2/relative/provider/coingecko/{coinIDs}/{vsCurrencies}/{duration}` request when it hits our servers:
1. First the app [attempts](https://github.com/brave-intl/bat-go/blob/44f73e740a2c2a277ffb621a4940a1330b524c6b/services/ratios/service.go#L154) to fetch the data from the cache
    1. It does an `HMGET "relative" {coinID_1} {coinID_2} {coinID_3}` to [fetch](https://github.com/brave-intl/bat-go/blob/44f73e740a2c2a277ffb621a4940a1330b524c6b/services/ratios/cache.go#L130-L138) price data from the cache for each of the coinIDs supplied.  This will return multiple cache entries, one for each coin ID.
    1. For each cache entry, the app
        1. Checks if the entry is empty or is too old, and if so [aborts](https://github.com/brave-intl/bat-go/blob/44f73e740a2c2a277ffb621a4940a1330b524c6b/services/ratios/cache.go#L153-L155) fetching data from the cache for all the coinIDs and returns an error.
        1. For each vs currency supplied, [checks](https://github.com/brave-intl/bat-go/blob/44f73e740a2c2a277ffb621a4940a1330b524c6b/services/ratios/cache.go#L158-L171) if the cache entry includes the vsCurrency expected and aborts fetching from the cache and returns an error if so.
        1. If all goes expected, the app plucks just the coinID and rate value data from the cache entry to create a new []SimplePriceResponse. Basically it strips the outer "payload", and "lastUpdated" keys.
    1. If there are no cache entries, the app [aborts](https://github.com/brave-intl/bat-go/blob/44f73e740a2c2a277ffb621a4940a1330b524c6b/services/ratios/cache.go#L173) fetching from the cache and returns an error
1. If the cache retrieval attempt returns an error, the app makes a [request](https://github.com/brave-intl/bat-go/blob/44f73e740a2c2a277ffb621a4940a1330b524c6b/services/ratios/service.go#L160) to Coingecko's `/simple/price` endpoint, suplying the coinIDs and vsCurrencies. Note: this FetchSimplePrice method on the Coingecko HTTP client does not cache the results after receiving the results - this could be a problem, or intentional.
1. Next the app handles the duration parameter and the market chart. The data from the price cache entry will always have 24 hour changes included. Since we support this duration parameter, we need to make an additional request to fetch it. But we only need to do so if there is exactly 1 coinID parameter. If the supplied duration the is not "1d", the app
    1. [alters](https://github.com/brave-intl/bat-go/blob/44f73e740a2c2a277ffb621a4940a1330b524c6b/services/ratios/service.go#L179-L189) the "change over duration" key by
        1. Renaming the key from "*_24h_change" -> "*_timeframe_change"
        1. Setting the value to 0
    1. If only 1 coin ID is supplied in the request, we [fetch](https://github.com/brave-intl/bat-go/blob/44f73e740a2c2a277ffb621a4940a1330b524c6b/services/ratios/service.go#L191-L208) the market chart to get the "change over duration" value
        1. TODO why?
        1. Note: the Coingecko http client will first check the cache for a market chart entry, and if not found will make the request to Coingecko `/coin/{id}/market_chart`, and then cache it.
        1. Note: we only fetch for the first vsCurrency

#### When is the cache read and written to?
* when does HMSET of price data happen

## notes

components/brave_wallet/browser/asset_ratio_service.h

* when does caching happen and by what mechanism?
  - RunNextRelativeCachePrepopulationJob run every 5 minutes
    - fetches top coins from the redis cache  (top 25)
    - fetch top currencies from redis cache   (top 5)
    - calls fetch simple for each (does it check the cache first? i don't think so)
    - Q: if there are more than 25, is there cache setting?
* can caching strategy be improved at all?
* can rate limiting strategy be improved at all?


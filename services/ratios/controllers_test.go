//go:build integration
// +build integration

package ratios_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/clients/coingecko"
	mockcoingecko "github.com/brave-intl/bat-go/libs/clients/coingecko/mock"
	ratiosclient "github.com/brave-intl/bat-go/libs/clients/ratios"
	"github.com/brave-intl/bat-go/libs/clients/stripe"
	mockstripe "github.com/brave-intl/bat-go/libs/clients/stripe/mock"
	appctx "github.com/brave-intl/bat-go/libs/context"
	logutils "github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/ratios"
	"github.com/go-chi/chi"
	"github.com/golang/mock/gomock"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type ControllersTestSuite struct {
	suite.Suite

	ctx                 context.Context
	service             *ratios.Service
	mockCtrl            *gomock.Controller
	mockCoingeckoClient *mockcoingecko.MockClient
	mockStripeClient    *mockstripe.MockClient
}

func TestControllersTestSuite(t *testing.T) {
	suite.Run(t, new(ControllersTestSuite))
}

func (suite *ControllersTestSuite) SetupSuite() {
	// setup the context
	suite.ctx = context.Background()

	// setup debug for client
	suite.ctx = context.WithValue(suite.ctx, appctx.DebugLoggingCTXKey, false)
	// setup debug log level
	suite.ctx = context.WithValue(suite.ctx, appctx.LogLevelCTXKey, "info")

	// setup a logger and put on context
	suite.ctx, _ = logutils.SetupLogger(suite.ctx)

	// setup server location
	suite.ctx = context.WithValue(suite.ctx, appctx.CoingeckoServerCTXKey, "https://api.coingecko.com")
	// setup token (using public api for tests)
	suite.ctx = context.WithValue(suite.ctx, appctx.CoingeckoAccessTokenCTXKey, "")
	// coin limit
	suite.ctx = context.WithValue(suite.ctx, appctx.CoingeckoCoinLimitCTXKey, 2)
	// vs-currency limit
	suite.ctx = context.WithValue(suite.ctx, appctx.CoingeckoVsCurrencyLimitCTXKey, 2)
	// all this is setup in init service
	suite.ctx = context.WithValue(suite.ctx, appctx.CoingeckoSymbolToIDCTXKey, map[string]string{"bat": "basic-attention-token"})
	suite.ctx = context.WithValue(suite.ctx, appctx.CoingeckoContractToIDCTXKey, map[string]string{})
	suite.ctx = context.WithValue(suite.ctx, appctx.CoingeckoIDToSymbolCTXKey, map[string]string{"basic-attention-token": "bat"})
	suite.ctx = context.WithValue(suite.ctx, appctx.CoingeckoSupportedVsCurrenciesCTXKey, map[string]bool{"usd": true})

	var redisAddr string = "redis://grant-redis:6379"
	if len(os.Getenv("REDIS_ADDR")) > 0 {
		redisAddr = os.Getenv("REDIS_ADDR")
	}

	suite.ctx = context.WithValue(suite.ctx, appctx.RatiosRedisAddrCTXKey, redisAddr)

	govalidator.SetFieldsRequiredByDefault(true)
}

func (suite *ControllersTestSuite) BeforeTest(sn, tn string) {
	suite.mockCtrl = gomock.NewController(suite.T())
	// setup a mock coingecko client
	redisAddr, err := appctx.GetStringFromContext(suite.ctx, appctx.RatiosRedisAddrCTXKey)
	suite.Require().NoError(err)

	opts, err := redis.ParseURL(redisAddr)
	suite.Require().NoError(err, "Must be able to parse redis URL")

	redis := redis.NewClient(opts)

	if err := redis.Ping(suite.ctx).Err(); err != nil {
		suite.Require().NoError(err, "Must be able to ping redis")
	}

	coingecko := mockcoingecko.NewMockClient(suite.mockCtrl)
	suite.mockCoingeckoClient = coingecko

	stripe := mockstripe.NewMockClient(suite.mockCtrl)
	suite.mockStripeClient = stripe

	suite.service = ratios.NewService(suite.ctx, coingecko, stripe, redis)
	suite.Require().NoError(err, "failed to setup ratios service")
}

func (suite *ControllersTestSuite) AfterTest(sn, tn string) {
	suite.mockCtrl.Finish()
}

func (suite *ControllersTestSuite) TestGetHistoryHandler() {
	handler := ratios.GetHistoryHandler(suite.service)
	req, err := http.NewRequest("GET", "/v2/history/coingecko/{coinID}/{vsCurrency}/{duration}", nil)
	suite.Require().NoError(err)

	// Test validation errors
	// ErrCoingeckoCoinInvalid
	req = req.WithContext(suite.ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("coinID", "invalidcoingeckocoin")
	rctx.URLParams.Add("vsCurrency", "usd")
	rctx.URLParams.Add("duration", "1d")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)
	suite.Require().Empty(rr.Header().Get("Cache-Control"))

	// ErrCoingeckoVsCurrencyInvalid
	req = req.WithContext(suite.ctx)
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("coinID", "bat")
	rctx.URLParams.Add("vsCurrency", "invalidcoingeckovscurrency")
	rctx.URLParams.Add("duration", "1d")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)
	suite.Require().Empty(rr.Header().Get("Cache-Control"))

	// ErrCoingeckoVsCurrencyEmpty
	req = req.WithContext(suite.ctx)
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("coinID", "bat")
	rctx.URLParams.Add("vsCurrency", "")
	rctx.URLParams.Add("duration", "1d")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)
	suite.Require().Empty(rr.Header().Get("Cache-Control"))

	// ErrCoingeckoDurationInvalid
	req = req.WithContext(suite.ctx)
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("coinID", "bat")
	rctx.URLParams.Add("vsCurrency", "usd")
	rctx.URLParams.Add("duration", "invalidcoingeckoduration")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)
	suite.Require().Empty(rr.Header().Get("Cache-Control"))

	// Test success with 1h duration
	suite.mockCoingeckoClient.EXPECT().
		FetchMarketChart(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&coingecko.MarketChartResponse{
			Prices: [][]decimal.Decimal{
				[]decimal.Decimal{decimal.Zero},
			},
		}, time.Now(), nil)
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("coinID", "bat")
	rctx.URLParams.Add("vsCurrency", "usd")
	rctx.URLParams.Add("duration", "1h")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)
	// example
	// {"payload":{"prices":[[...
	var resp = new(ratios.HistoryResponse)
	err = json.Unmarshal(rr.Body.Bytes(), resp)
	suite.Require().NoError(err)
	suite.Require().True(len(resp.Payload.Prices) > 0)
	cacheControl := rr.Header().Get("Cache-Control")
	suite.Require().NotEmpty(cacheControl, "Cache-Control header is not present")
	maxAgeRegex := regexp.MustCompile(`max-age=(\d+)`)
	maxAgeMatch := maxAgeRegex.FindStringSubmatch(cacheControl)
	suite.Require().Len(maxAgeMatch, 2, "Invalid max-age parameter in Cache-Control header")
	maxAge, err := strconv.Atoi(maxAgeMatch[1])
	suite.Require().Greater(maxAge, 0, "Invalid max-age parameter in Cache-Control header")
	suite.Require().LessOrEqual(maxAge, 150, "Invalid max-age parameter in Cache-Control header")

	// Test success with 1d duration
	suite.mockCoingeckoClient.EXPECT().
		FetchMarketChart(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&coingecko.MarketChartResponse{
			Prices: [][]decimal.Decimal{
				[]decimal.Decimal{decimal.Zero},
			},
		}, time.Now(), nil)
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("coinID", "bat")
	rctx.URLParams.Add("vsCurrency", "usd")
	rctx.URLParams.Add("duration", "1d")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)
	var resp1d = new(ratios.HistoryResponse)
	err = json.Unmarshal(rr.Body.Bytes(), resp1d)
	suite.Require().NoError(err)
	suite.Require().True(len(resp1d.Payload.Prices) > 0)
	cacheControl = rr.Header().Get("Cache-Control")
	suite.Require().NotEmpty(cacheControl, "Cache-Control header is not present")
	maxAgeRegex = regexp.MustCompile(`max-age=(\d+)`)
	maxAgeMatch = maxAgeRegex.FindStringSubmatch(cacheControl)
	suite.Require().Len(maxAgeMatch, 2, "Invalid max-age parameter in Cache-Control header")
	maxAge, err = strconv.Atoi(maxAgeMatch[1])
	suite.Require().Greater(maxAge, 150, "Invalid max-age parameter in Cache-Control header")
	suite.Require().LessOrEqual(maxAge, 3600, "Invalid max-age parameter in Cache-Control header")

	// Test success with 1w duration
	suite.mockCoingeckoClient.EXPECT().
		FetchMarketChart(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&coingecko.MarketChartResponse{
			Prices: [][]decimal.Decimal{
				[]decimal.Decimal{decimal.Zero},
			},
		}, time.Now(), nil)
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("coinID", "bat")
	rctx.URLParams.Add("vsCurrency", "usd")
	rctx.URLParams.Add("duration", "1w")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)
	var resp1w = new(ratios.HistoryResponse)
	err = json.Unmarshal(rr.Body.Bytes(), resp1w)
	suite.Require().NoError(err)
	suite.Require().True(len(resp1w.Payload.Prices) > 0)
	cacheControl = rr.Header().Get("Cache-Control")
	suite.Require().NotEmpty(cacheControl, "Cache-Control header is not present")
	maxAgeRegex = regexp.MustCompile(`max-age=(\d+)`)
	maxAgeMatch = maxAgeRegex.FindStringSubmatch(cacheControl)
	suite.Require().Len(maxAgeMatch, 2, "Invalid max-age parameter in Cache-Control header")
	maxAge, err = strconv.Atoi(maxAgeMatch[1])
	suite.Require().Greater(maxAge, 3600, "Invalid max-age parameter in Cache-Control header")
	suite.Require().LessOrEqual(maxAge, 25200, "Invalid max-age parameter in Cache-Control header")

	// Test success with 1m, 3m, 1y, all durations.
	// They should all set a maximum cache-control header of 1 day (86400 seconds)
	durations := []string{"1m", "3m", "1y", "all"}

	for _, duration := range durations {
		suite.mockCoingeckoClient.EXPECT().
			FetchMarketChart(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&coingecko.MarketChartResponse{
				Prices: [][]decimal.Decimal{
					[]decimal.Decimal{decimal.Zero},
				},
			}, time.Now(), nil)
		rctx = chi.NewRouteContext()
		rctx.URLParams.Add("coinID", "bat")
		rctx.URLParams.Add("vsCurrency", "usd")
		rctx.URLParams.Add("duration", duration)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rr = httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		suite.Require().Equal(http.StatusOK, rr.Code)
		var resp = new(ratios.HistoryResponse)
		err = json.Unmarshal(rr.Body.Bytes(), resp)
		suite.Require().NoError(err)
		suite.Require().True(len(resp.Payload.Prices) > 0)
		cacheControl = rr.Header().Get("Cache-Control")
		suite.Require().NotEmpty(cacheControl, "Cache-Control header is not present")
		maxAgeRegex = regexp.MustCompile(`max-age=(\d+)`)
		maxAgeMatch = maxAgeRegex.FindStringSubmatch(cacheControl)
		suite.Require().Len(maxAgeMatch, 2, "Invalid max-age parameter in Cache-Control header")
		maxAge, err = strconv.Atoi(maxAgeMatch[1])
		suite.Require().Greater(maxAge, 25200, "Invalid max-age parameter in Cache-Control header")
		suite.Require().LessOrEqual(maxAge, 86400, "Invalid max-age parameter in Cache-Control header")
	}
}

func (suite *ControllersTestSuite) TestGetRelativeHandler() {
	handler := ratios.GetRelativeHandler(suite.service)
	// Test validation errors
	// ErrCoingeckoCoinInvalid
	req, err := http.NewRequest("GET", "/v2/relative/provider/coingecko/{coinIDs}/{vsCurrencies}/{duration}", nil)
	suite.Require().NoError(err)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("coinIDs", "invalidcoingeckocoin")
	rctx.URLParams.Add("vsCurrencies", "usd")
	rctx.URLParams.Add("duration", "1d")
	req = req.WithContext(suite.ctx)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)
	suite.Require().Empty(rr.Header().Get("Cache-Control"))

	// ErrCoingeckoCoinEmpty
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("coinIDs", "")
	rctx.URLParams.Add("vsCurrencies", "usd")
	rctx.URLParams.Add("duration", "1d")
	req = req.WithContext(suite.ctx)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)
	suite.Require().Empty(rr.Header().Get("Cache-Control"))

	// ErrCoingeckoCoinListLimit
	rctx = chi.NewRouteContext()
	coinIDs := "one,celo,sgb,kcs,xdai,metis,wan,bnb,okt,vs,fitfi,tfuel,klay,bch,mtr,eth,kava,fsn,glmr,cro,canto,avax,iotx,doge,rbtc,rose,xdc,fuse,tt,vlx,brise,ulx,evmos,sol,matic,ftm,cet,tlos"
	rctx.URLParams.Add("coinIDs", coinIDs)
	rctx.URLParams.Add("vsCurrencies", "usd")
	rctx.URLParams.Add("duration", "1d")
	req = req.WithContext(suite.ctx)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)
	suite.Require().Empty(rr.Header().Get("Cache-Control"))

	// ErrCoingeckoVsCurrencyEmpty
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("coinIDs", "bat")
	rctx.URLParams.Add("vsCurrencies", "")
	rctx.URLParams.Add("duration", "1d")
	req = req.WithContext(suite.ctx)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)
	suite.Require().Empty(rr.Header().Get("Cache-Control"))

	// ErrCoingeckoVsCurrencyLimit
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("coinIDs", "bat")
	rctx.URLParams.Add("vsCurrencies", coinIDs)
	rctx.URLParams.Add("duration", "1d")
	req = req.WithContext(suite.ctx)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)
	suite.Require().Empty(rr.Header().Get("Cache-Control"))

	// ErrCoingeckoDurationInvalid
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("coinIDs", "bat")
	rctx.URLParams.Add("vsCurrencies", "usd")
	rctx.URLParams.Add("duration", "invalidcoingeckoduration")
	req = req.WithContext(suite.ctx)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)
	suite.Require().Empty(rr.Header().Get("Cache-Control"))

	// Test success case
	respy := coingecko.SimplePriceResponse(map[string]map[string]decimal.Decimal{
		"basic-attention-token": map[string]decimal.Decimal{"usd": decimal.Zero},
	})
	suite.mockCoingeckoClient.EXPECT().
		FetchSimplePrice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(
			&respy, nil)
	req, err = http.NewRequest("GET", "/v2/relative/provider/coingecko/{coinIDs}/{vsCurrencies}/{duration}", nil)
	suite.Require().NoError(err)

	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("coinIDs", "bat")
	rctx.URLParams.Add("vsCurrencies", "usd")
	rctx.URLParams.Add("duration", "1d")

	// add in our suite ctx
	req = req.WithContext(suite.ctx)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	// validate response code matches
	suite.Require().Equal(http.StatusOK, rr.Code)
	// validate cache control header is set and is correct
	cacheControl := rr.Header().Get("Cache-Control")
	suite.Require().NotEmpty(cacheControl, "Cache-Control header is not present")
	maxAgeRegex := regexp.MustCompile(`max-age=(\d+)`)
	maxAgeMatch := maxAgeRegex.FindStringSubmatch(cacheControl)
	suite.Require().Len(maxAgeMatch, 2, "Invalid max-age parameter in Cache-Control header")
	maxAge, err := strconv.Atoi(maxAgeMatch[1])
	suite.Require().NoError(err, "Invalid max-age parameter in Cache-Control header")
	suite.Require().Greater(maxAge, 0, "Invalid max-age parameter in Cache-Control header")
	suite.Require().LessOrEqual(maxAge, 900, "Invalid max-age parameter in Cache-Control header")

	// example
	// {"payload":{"bat":{"usd":1.3,"usd_timeframe_change":0.8356218194962891}}
	var resp = new(ratiosclient.RelativeResponse)
	err = json.Unmarshal(rr.Body.Bytes(), resp)
	suite.Require().NoError(err)

	v, okBat := resp.Payload["bat"]
	suite.Require().True(okBat)

	_, okUsd := v["usd"]
	suite.Require().True(okUsd)
}

func (suite *ControllersTestSuite) TestGetCoinMarketsHandler() {
	handler := ratios.GetCoinMarketsHandler(suite.service)
	coingeckoResp := coingecko.CoinMarketResponse(
		[]*coingecko.CoinMarket{
			{
				ID:                       "bitcoin",
				Symbol:                   "btc",
				Name:                     "Bitcoin",
				Image:                    "https://assets.coingecko.com/coins/images/1/large/bitcoin.png?1547033579",
				MarketCap:                728577821016,
				MarketCapRank:            1,
				CurrentPrice:             38400,
				PriceChange24h:           558.39,
				PriceChangePercentage24h: 1.4756,
				TotalVolume:              41369367560,
			},
		},
	)
	suite.mockCoingeckoClient.EXPECT().
		FetchCoinMarkets(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&coingeckoResp, time.Now(), nil)
	req, err := http.NewRequest("GET", "/v2/market/provider/coingecko?vsCurrency=usd&limit=1", nil)
	suite.Require().NoError(err)
	rctx := chi.NewRouteContext()
	req = req.WithContext(suite.ctx)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)
	var resp = new(ratios.GetCoinMarketsResponse)
	err = json.Unmarshal(rr.Body.Bytes(), resp)
	suite.Require().NoError(err)
	suite.Require().Equal(len(resp.Payload), 1)
	suite.Require().Equal(resp.Payload[0].Symbol, "btc")
	cacheControl := rr.Header().Get("Cache-Control")
	suite.Require().NotEmpty(cacheControl, "Cache-Control header is not present")
	maxAgeRegex := regexp.MustCompile(`max-age=(\d+)`)
	maxAgeMatch := maxAgeRegex.FindStringSubmatch(cacheControl)
	suite.Require().Len(maxAgeMatch, 2, "Invalid max-age parameter in Cache-Control header")
	maxAge, err := strconv.Atoi(maxAgeMatch[1])
	suite.Require().Greater(maxAge, 0, "Invalid max-age parameter in Cache-Control header")
	suite.Require().LessOrEqual(maxAge, 3600, "Invalid max-age parameter in Cache-Control header")
}

func (suite *ControllersTestSuite) TestCreateStripeOnrampSessionsHandler() {
	handler := ratios.CreateStripeOnrampSessionsHandler(suite.service)
	// Missing payload results in 400
	{
		req, err := http.NewRequest("POST", "/v2/stripe/onramp_sessions", nil)
		suite.Require().NoError(err)
		rctx := chi.NewRouteContext()
		req = req.WithContext(suite.ctx)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		suite.Require().Equal(http.StatusBadRequest, rr.Code)
	}

	// SourceExchangeAmount less than 1 results in 400
	{
		payload := &ratios.StripeOnrampSessionRequest{
			WalletAddress:                "0x123abc456def",
			SourceCurrency:               "usd",
			SourceExchangeAmount:         "0.5",
			DestinationNetwork:           "ethereum",
			DestinationCurrency:          "eth",
			SupportedDestinationNetworks: []string{"ethereum", "bitcoin", "solana", "polygon"},
		}
		payloadBytes, err := json.Marshal(payload)
		suite.Require().NoError(err)
		req, err := http.NewRequest("POST", "/v2/stripe/onramp_sessions", bytes.NewBuffer(payloadBytes))
		suite.Require().NoError(err)
		rctx := chi.NewRouteContext()
		req = req.WithContext(suite.ctx)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		suite.Require().Equal(http.StatusBadRequest, rr.Code)
	}

	// SourceExchangeAmount includes fractions of pennies results in 400
	{
		payload := &ratios.StripeOnrampSessionRequest{
			WalletAddress:                "0x123abc456def",
			SourceCurrency:               "usd",
			SourceExchangeAmount:         "1000.001",
			DestinationNetwork:           "ethereum",
			DestinationCurrency:          "eth",
			SupportedDestinationNetworks: []string{"ethereum", "bitcoin", "solana", "polygon"},
		}
		payloadBytes, err := json.Marshal(payload)
		suite.Require().NoError(err)
		req, err := http.NewRequest("POST", "/v2/stripe/onramp_sessions", bytes.NewBuffer(payloadBytes))
		suite.Require().NoError(err)
		rctx := chi.NewRouteContext()
		req = req.WithContext(suite.ctx)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		suite.Require().Equal(http.StatusBadRequest, rr.Code)
	}

	// Invalid DestinationNetwork results in 400
	{
		payload := &ratios.StripeOnrampSessionRequest{
			WalletAddress:                "0x123abc456def",
			SourceCurrency:               "USD",
			SourceExchangeAmount:         "1000.00",
			DestinationNetwork:           "unsupportedNetwork",
			DestinationCurrency:          "ETH",
			SupportedDestinationNetworks: []string{"ethereum", "bitcoin", "solana", "polygon"},
		}
		payloadBytes, err := json.Marshal(payload)
		suite.Require().NoError(err)
		req, err := http.NewRequest("POST", "/v2/stripe/onramp_sessions", bytes.NewBuffer(payloadBytes))
		suite.Require().NoError(err)
		rctx := chi.NewRouteContext()
		req = req.WithContext(suite.ctx)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		suite.Require().Equal(http.StatusBadRequest, rr.Code)
	}

	// Unsupported network in SupportedDestinationNetworks results in 400
	{
		payload := &ratios.StripeOnrampSessionRequest{
			WalletAddress:                "0x123abc456def",
			SourceCurrency:               "usd",
			SourceExchangeAmount:         "1000.00",
			DestinationNetwork:           "ethereum",
			DestinationCurrency:          "eth",
			SupportedDestinationNetworks: []string{"ethereum", "binance", "cardano"},
		}
		payloadBytes, err := json.Marshal(payload)
		suite.Require().NoError(err)
		req, err := http.NewRequest("POST", "/v2/stripe/onramp_sessions", bytes.NewBuffer(payloadBytes))
		suite.Require().NoError(err)
		rctx := chi.NewRouteContext()
		req = req.WithContext(suite.ctx)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		suite.Require().Equal(http.StatusBadRequest, rr.Code)
	}

	// Invalid DestinationCurrency results in 400
	{
		payload := &ratios.StripeOnrampSessionRequest{
			WalletAddress:                "0x123abc456def",
			SourceCurrency:               "usd",
			SourceExchangeAmount:         "1000.00",
			DestinationNetwork:           "ethereum",
			DestinationCurrency:          "unsupportedCurrency",
			SupportedDestinationNetworks: []string{"ethereum", "bitcoin", "solana", "polygon"},
		}
		payloadBytes, err := json.Marshal(payload)
		suite.Require().NoError(err)
		req, err := http.NewRequest("POST", "/v2/stripe/onramp_sessions", bytes.NewBuffer(payloadBytes))
		suite.Require().NoError(err)
		rctx := chi.NewRouteContext()
		req = req.WithContext(suite.ctx)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		suite.Require().Equal(http.StatusBadRequest, rr.Code)
	}

	// Valid request yields 200
	{
		stripeResp := stripe.OnrampSessionResponse{
			RedirectURL: "https://example.com",
		}
		suite.mockStripeClient.EXPECT().
			CreateOnrampSession(
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			).
			Return(&stripeResp, nil)
		payload := &ratios.StripeOnrampSessionRequest{
			WalletAddress:                "0x123abc456def",
			SourceCurrency:               "usd",
			SourceExchangeAmount:         "1000.00",
			DestinationNetwork:           "ethereum",
			DestinationCurrency:          "eth",
			SupportedDestinationNetworks: []string{"ethereum", "solana", "bitcoin"},
		}
		payloadBytes, err := json.Marshal(payload)
		suite.Require().NoError(err)
		req, err := http.NewRequest("POST", "/v2/stripe/onramp_sessions", bytes.NewBuffer(payloadBytes))
		suite.Require().NoError(err)
		rctx := chi.NewRouteContext()
		req = req.WithContext(suite.ctx)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		suite.Require().Equal(http.StatusOK, rr.Code)
		var ratiosResp = new(ratios.CreateStripeOnrampSessionResponse)
		err = json.Unmarshal(rr.Body.Bytes(), ratiosResp)
		suite.Require().NoError(err)
		suite.Require().Equal(ratiosResp.URL, "https://example.com")
	}
}

//go:build integration

package coingecko_test

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/clients/coingecko"
	appctx "github.com/brave-intl/bat-go/libs/context"
	logutils "github.com/brave-intl/bat-go/libs/logging"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
)

type CoingeckoTestSuite struct {
	suite.Suite
	redis  *redis.Client
	client coingecko.Client
	ctx    context.Context
}

func TestCoingeckoTestSuite(t *testing.T) {
	suite.Run(t, new(CoingeckoTestSuite))
}

var (
	coingeckoService       string = "https://api.coingecko.com/"
	coingeckoToken         string
	coingeckoCoinLimit     int = 2
	coingeckoCurrencyLimit int = 2
)

func (suite *CoingeckoTestSuite) SetupTest() {
	// setup the context
	suite.ctx = context.Background()

	// debug logging generates too much noise, so do not activate it.
	suite.ctx = context.WithValue(suite.ctx, appctx.DebugLoggingCTXKey, false)

	// setup a logger and put on context
	suite.ctx, _ = logutils.SetupLogger(suite.ctx)

	// setup server location
	suite.ctx = context.WithValue(suite.ctx, appctx.CoingeckoServerCTXKey, coingeckoService)
	// setup token (using public api for tests)
	suite.ctx = context.WithValue(suite.ctx, appctx.CoingeckoAccessTokenCTXKey, coingeckoToken)
	// coin limit
	suite.ctx = context.WithValue(suite.ctx, appctx.CoingeckoCoinLimitCTXKey, coingeckoCoinLimit)
	// vs-currency limit
	suite.ctx = context.WithValue(suite.ctx, appctx.CoingeckoVsCurrencyLimitCTXKey, coingeckoCurrencyLimit)

	redisAddr := "redis://grant-redis:6379/1"
	if len(os.Getenv("REDIS_ADDR")) > 0 {
		redisAddr = os.Getenv("REDIS_ADDR")
	}

	opts, err := redis.ParseURL(redisAddr)
	suite.Require().NoError(err, "Must be able to parse redis URL")
	suite.redis = redis.NewClient(opts)

	err = suite.redis.Ping(suite.ctx).Err()
	suite.Require().NoError(err, "Must be able to connect to redis")

	// setup the client under test, no redis, will test redis interactions in ratios service
	suite.client, err = coingecko.NewWithContext(suite.ctx, suite.redis)
	suite.Require().NoError(err, "Must be able to correctly initialize the client")
}

func (suite *CoingeckoTestSuite) TearDownTest() {
	// flush all keys from the test Redis database
	suite.Assert().NoError(suite.redis.FlushDB(suite.ctx).Err(), "Must be able to flush Redis database")
	// work around Coingecko rate limit
	time.Sleep(200 * time.Millisecond)
}

func (suite *CoingeckoTestSuite) TestFetchSimplePrice() {
	resp, err := suite.client.FetchSimplePrice(suite.ctx, "basic-attention-token", "usd", false)
	suite.Require().NoError(err, "should be able to fetch the simple price")

	// simple price response should have a bat key
	_, ok := (*resp)["basic-attention-token"]
	suite.Require().True(ok, "should have bat in the response")
}

func (suite *CoingeckoTestSuite) TestFetchCoinList() {
	resp, err := suite.client.FetchCoinList(suite.ctx, false)
	suite.Require().NoError(err, "should be able to fetch the coin list")

	// simple price response should have a bat key
	var foundBAT bool
	for _, v := range *resp {
		if v.ID == "basic-attention-token" {
			foundBAT = true
			break
		}
	}
	suite.Require().True(foundBAT, "should have bat in the response")
}

func (suite *CoingeckoTestSuite) TestFetchSupportedVsCurrencies() {
	resp, err := suite.client.FetchSupportedVsCurrencies(suite.ctx)
	suite.Require().NoError(err, "should be able to fetch the vs_currency list")

	// simple price response should have a bat key
	var foundUSD bool
	for _, v := range *resp {
		if v == "usd" {
			foundUSD = true
			break
		}
	}
	suite.Require().True(foundUSD, "should have usd in the response")
}

func (suite *CoingeckoTestSuite) TestFetchMarketChart() {
	resp, t, err := suite.client.FetchMarketChart(suite.ctx, "basic-attention-token", "usd", 1.0, 3600)
	suite.Require().NoError(err, "should be able to fetch the market chart")

	// call again, make sure you get back the same resp and t
	resp1, t1, err := suite.client.FetchMarketChart(suite.ctx, "basic-attention-token", "usd", 1.0, 3600)
	suite.Require().NoError(err, "should be able to fetch the market chart")

	b, err := json.Marshal(resp)
	suite.Require().NoError(err, "should marshal first resp")

	b1, err := json.Marshal(resp1)
	suite.Require().NoError(err, "should marshal second resp")

	suite.Require().True(string(b) == string(b1) && t1.Equal(t), "didn't use cached response")

}

func (suite *CoingeckoTestSuite) TestFetchCoinMarkets() {
	resp, t, err := suite.client.FetchCoinMarkets(suite.ctx, "usd", 1)
	suite.Require().NoError(err, "should be able to fetch the coin markets")
	suite.Require().Equal(1, len(*resp), "should have a response length of 1 for limit=1")
	suite.Require().NotNil((*resp)[0].CurrentPrice, "should have a value for price")
	suite.Require().NotEqual((*resp)[0].CurrentPrice, 0, "bitcoin is never going to 0")

	// call again but with biggger limit
	// in this case we should have more results, but only used cached response from redis
	resp1, t1, err := suite.client.FetchCoinMarkets(suite.ctx, "usd", 10)
	suite.Require().NoError(err, "should be able to fetch the coin markets")
	suite.Require().Equal(10, len(*resp1), "should have a response length of 10 for limit=10")
	suite.Require().Equal(t.Unix(), t1.Unix(), "the lastUpdated time should be equal because of cache usage")
	u, err := url.Parse((*resp1)[0].Image)
	suite.Require().NoError(err)
	suite.Require().Equal(u.Host, "assets.cgproxy.brave.com", "image host should be the brave proxy")
}

//go:build integration && vpn
// +build integration,vpn

package coingecko_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/utils/clients/coingecko"
	appctx "github.com/brave-intl/bat-go/utils/context"
	logutils "github.com/brave-intl/bat-go/utils/logging"
	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/suite"
)

type CoingeckoTestSuite struct {
	suite.Suite
	redisPool *redis.Pool
	client    coingecko.Client
	ctx       context.Context
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

	// setup debug for client
	suite.ctx = context.WithValue(suite.ctx, appctx.DebugLoggingCTXKey, false)
	// setup debug log level
	suite.ctx = context.WithValue(suite.ctx, appctx.LogLevelCTXKey, "info")

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

	var redisAddr string = "redis://grant-redis"
	if len(os.Getenv("REDIS_ADDR")) > 0 {
		redisAddr = os.Getenv("REDIS_ADDR")
	}

	suite.redisPool = &redis.Pool{
		MaxIdle:   50,
		MaxActive: 1000,
		Dial: func() (redis.Conn, error) {
			conn, err := redis.DialURL(redisAddr)
			suite.Require().NoError(err, "failed to connect to redis")
			return conn, err
		},
	}

	rConn := suite.redisPool.Get()
	defer rConn.Close()
	s, err := redis.String(rConn.Do("PING"))
	suite.Require().NoError(err, "failed to connect to redis")
	suite.Require().True(s == "PONG", "bad response from redis")

	// setup the client under test, no redis, will test redis interactions in ratios service
	suite.client, err = coingecko.NewWithContext(suite.ctx, suite.redisPool)
	suite.Require().NoError(err, "Must be able to correctly initialize the client")

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
	resp, t, err := suite.client.FetchMarketChart(suite.ctx, "basic-attention-token", "usd", 1.0)
	suite.Require().NoError(err, "should be able to fetch the market chart")

	// call again, make sure you get back the same resp and t
	resp1, t1, err := suite.client.FetchMarketChart(suite.ctx, "basic-attention-token", "usd", 1.0)
	suite.Require().NoError(err, "should be able to fetch the market chart")

	b, err := json.Marshal(resp)
	suite.Require().NoError(err, "should marshal first resp")

	b1, err := json.Marshal(resp1)
	suite.Require().NoError(err, "should marshal second resp")

	suite.Require().True(string(b) == string(b1) && t1.Equal(t), "didn't use cached response")
}

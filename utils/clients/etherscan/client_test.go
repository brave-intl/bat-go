//go:build integration && vpn
// +build integration,vpn

package etherscan_test

import (
	"context"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/utils/clients/etherscan"
	appctx "github.com/brave-intl/bat-go/utils/context"
	logutils "github.com/brave-intl/bat-go/utils/logging"
	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/suite"
)

type etherscanTestSuite struct {
	suite.Suite
	redisPool *redis.Pool
	client    etherscan.Client
	ctx       context.Context
}

func TestetherscanTestSuite(t *testing.T) {
	suite.Run(t, new(etherscanTestSuite))
}

var (
	etherscanService       string = "https://api.etherscan.com/"
	etherscanToken         string
	etherscanCoinLimit     int = 2
	etherscanCurrencyLimit int = 2
)

func (suite *etherscanTestSuite) SetupTest() {
	// setup the context
	suite.ctx = context.Background()

	// setup debug for client
	suite.ctx = context.WithValue(suite.ctx, appctx.DebugLoggingCTXKey, false)
	// setup debug log level
	suite.ctx = context.WithValue(suite.ctx, appctx.LogLevelCTXKey, "info")

	// setup a logger and put on context
	suite.ctx, _ = logutils.SetupLogger(suite.ctx)

	// setup server location
	suite.ctx = context.WithValue(suite.ctx, appctx.EtherscanURICTXKey, etherscanService)
	// setup token (using public api for tests)
	suite.ctx = context.WithValue(suite.ctx, appctx.EtherscanTokenCTXKey, etherscanToken)

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
	suite.client, err = etherscan.NewWithContext(suite.ctx, suite.redisPool)
	suite.Require().NoError(err, "Must be able to correctly initialize the client")
}

func (suite *etherscanTestSuite) TestFetchGasOracle() {
	resp, t, err := suite.client.FetchGasOracle(suite.ctx)
	suite.Require().NoError(err, "should be able to fetch the gas oracle")
}

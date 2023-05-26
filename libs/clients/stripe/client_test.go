//go:build integration && vpn
// +build integration,vpn

package stripe_test

import (
	"context"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/libs/clients/stripe"
	appctx "github.com/brave-intl/bat-go/libs/context"
	logutils "github.com/brave-intl/bat-go/libs/logging"
	"github.com/stretchr/testify/suite"
)

type StripeTestSuite struct {
	suite.Suite
	client stripe.Client
	ctx    context.Context
}

func TestStripeTestSuite(t *testing.T) {
	if _, exists := os.LookupEnv("STRIPE_ONRAMP_SECRET_KEY"); !exists {
		t.Skip("STRIPE_ONRAMP_SECRET_KEY is not found, skipping all tests in StripeTestSuite.")
	}

	suite.Run(t, new(StripeTestSuite))
}

var (
	stripeService string = "https://api.stripe.com/"
)

func (suite *StripeTestSuite) SetupTest() {
	// setup the context
	suite.ctx = context.Background()
	suite.ctx = context.WithValue(suite.ctx, appctx.DebugLoggingCTXKey, false)
	suite.ctx = context.WithValue(suite.ctx, appctx.LogLevelCTXKey, "info")
	suite.ctx, _ = logutils.SetupLogger(suite.ctx)

	stripeKey := os.Getenv("STRIPE_ONRAMP_SECRET_KEY")

	// Set stripeKey and stripeService into context
	suite.ctx = context.WithValue(suite.ctx, appctx.StripeOnrampServerCTXKey, stripeService)
	suite.ctx = context.WithValue(suite.ctx, appctx.StripeOnrampSecretKeyCTXKey, stripeKey)

	var err error
	suite.client, err = stripe.NewWithContext(suite.ctx)
	suite.Require().NoError(err, "Must be able to correctly initialize the client")
}

func (suite *StripeTestSuite) TestCreateOnrampSession() {
	// Empty params should yield a redirect URL
	var walletAddress string
	var sourceCurrency string
	var sourceExchangeAmount string
	var destinationNetwork string
	var destinationCurrency string
	var supportedDestinationNetworks []string

	resp, err := suite.client.CreateOnrampSession(
		suite.ctx,
		"redirect",
		walletAddress,
		sourceCurrency,
		sourceExchangeAmount,
		destinationNetwork,
		destinationCurrency,
		supportedDestinationNetworks,
	)
	suite.Require().NoError(err, "should be able to create an onramp session with no params")
	suite.Require().NotEqual(resp.RedirectURL, "")

	// Filled out params should yield a redirect URL
	walletAddress = "0xB00F0759DbeeF5E543Cc3E3B07A6442F5f3928a2"
	sourceCurrency = "usd"
	destinationCurrency = "eth"
	destinationNetwork = "ethereum"
	sourceExchangeAmount = "1"
	supportedDestinationNetworks = []string{"ethereum", "polygon"}
	resp, err = suite.client.CreateOnrampSession(
		suite.ctx,
		"redirect",
		walletAddress,
		sourceCurrency,
		sourceExchangeAmount,
		destinationNetwork,
		destinationCurrency,
		supportedDestinationNetworks,
	)
	suite.Require().NoError(err, "should be able to create an onramp session with specific params")
	suite.Require().NotEqual(resp.RedirectURL, "")
}

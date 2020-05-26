package cmd

import (
	"context"
	"net/http"
	"time"

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/rewards"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RewardsRestRun - Main entrypoint of the REST subcommand
// This function takes a cobra command and starts up the
// rewards rest microservice.
func RewardsRestRun(cmd *cobra.Command, args []string) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}

	// add profiling flag to enable profiling routes
	if viper.GetString("pprof-enabled") != "" {
		// pprof attaches routes to default serve mux
		// host:6061/debug/pprof/
		go func() {
			logger.Error().Err(http.ListenAndServe(":6061", http.DefaultServeMux))
		}()
	}

	// add our command line params to context
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, viper.Get("environment"))
	ctx = context.WithValue(ctx, appctx.RatiosServerCTXKey, viper.Get("ratios-service"))
	ctx = context.WithValue(ctx, appctx.RatiosAccessTokenCTXKey, viper.Get("ratios-token"))
	ctx = context.WithValue(ctx, appctx.BaseCurrencyCTXKey, viper.Get("base-currency"))

	// parse default-monthly-choices and default-tip-choices

	var monthlyChoices = []float64{}
	if err := viper.UnmarshalKey("default-monthly-choices", &monthlyChoices); err != nil {
		logger.Fatal().Err(err).Msg("failed to parse default-monthly-choices")
	}
	ctx = context.WithValue(ctx, appctx.DefaultMonthlyChoicesCTXKey, monthlyChoices)

	var tipChoices = []float64{}
	if err := viper.UnmarshalKey("default-tip-choices", &tipChoices); err != nil {
		logger.Fatal().Err(err).Msg("failed to parse default-tip-choices")
	}
	ctx = context.WithValue(ctx, appctx.DefaultTipChoicesCTXKey, tipChoices)

	// setup the service now
	s, err := rewards.InitService(ctx)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initalize rewards service")
	}

	// do rest endpoints
	r := setupRouter(ctx)
	r.Get("/v1/parameters", middleware.InstrumentHandler(
		"GetParametersHandler", rewards.GetParametersHandler(s)).ServeHTTP)

	// setup server, and run
	srv := http.Server{
		Addr:         viper.GetString("address"),
		Handler:      chi.ServerBaseContext(ctx, r),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 20 * time.Second,
	}

	// make sure exceptions go to sentry
	defer sentry.Flush(time.Second * 2)

	if err = srv.ListenAndServe(); err != nil {
		sentry.CaptureException(err)
		logger.Fatal().Err(err).Msg("HTTP server start failed!")
	}
}

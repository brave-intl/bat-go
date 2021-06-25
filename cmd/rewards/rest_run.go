package rewards

import (
	"context"
	"net/http"
	"time"

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/rewards"
	appctx "github.com/brave-intl/bat-go/utils/context"
	sentry "github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RestRun - Main entrypoint of the REST subcommand
// This function takes a cobra command and starts up the
// rewards rest microservice.
func RestRun(command *cobra.Command, args []string) {
	ctx, logger := CommonRun(command, args)
	// add profiling flag to enable profiling routes
	if viper.GetString("pprof-enabled") != "" {
		// pprof attaches routes to default serve mux
		// host:6061/debug/pprof/
		go func() {
			logger.Error().Err(http.ListenAndServe(":6061", http.DefaultServeMux))
		}()
	}

	// add our command line params to context
	ctx = context.WithValue(ctx, appctx.RatiosServerCTXKey, viper.Get("ratios-service"))
	ctx = context.WithValue(ctx, appctx.RatiosAccessTokenCTXKey, viper.Get("ratios-token"))
	ctx = context.WithValue(ctx, appctx.BaseCurrencyCTXKey, viper.Get("base-currency"))
	ctx = context.WithValue(ctx, appctx.RatiosCacheExpiryDurationCTXKey, viper.GetDuration("ratios-client-cache-expiry"))
	ctx = context.WithValue(ctx, appctx.RatiosCachePurgeDurationCTXKey, viper.GetDuration("ratios-client-cache-purge"))
	ctx = context.WithValue(ctx, appctx.DefaultACChoiceCTXKey, viper.GetFloat64("default-ac-choice"))

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

	var acChoices = []float64{}
	if err := viper.UnmarshalKey("default-ac-choices", &acChoices); err != nil {
		logger.Fatal().Err(err).Msg("failed to parse default-ac-choices")
	}
	if len(acChoices) > 0 {
		ctx = context.WithValue(ctx, appctx.DefaultACChoicesCTXKey, acChoices)
	}

	// setup the service now
	s, err := rewards.InitService(ctx)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initalize rewards service")
	}

	// do rest endpoints
	r := cmd.SetupRouter(command.Context())
	r.Get("/v1/parameters", middleware.InstrumentHandler(
		"GetParametersHandler", rewards.GetParametersHandler(s)).ServeHTTP)

	// make sure exceptions go to sentry
	defer sentry.Flush(time.Second * 2)

	go func() {
		err := http.ListenAndServe(":9090", middleware.Metrics())
		if err != nil {
			sentry.CaptureException(err)
			logger.Panic().Err(err).Msg("metrics HTTP server start failed!")
		}
	}()

	// setup server, and run
	srv := http.Server{
		Addr:         viper.GetString("address"),
		Handler:      chi.ServerBaseContext(ctx, r),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 20 * time.Second,
	}

	if err = srv.ListenAndServe(); err != nil {
		sentry.CaptureException(err)
		logger.Fatal().Err(err).Msg("HTTP server start failed!")
	}
}

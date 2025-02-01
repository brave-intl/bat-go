package cmd

import (
	"context"
	"fmt"
	"net/http"
	// pprof imports
	_ "net/http/pprof"
	"os"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cmdutils "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/services/cmd"
	"github.com/brave-intl/bat-go/services/rewards"
	"github.com/brave-intl/bat-go/services/rewards/handler"
)

// RestRun - Main entrypoint of the REST subcommand
// This function takes a cobra command and starts up the
// rewards rest microservice.
func RestRun(command *cobra.Command, args []string) {
	ctx := command.Context()
	lg, err := appctx.GetLogger(ctx)
	cmdutils.Must(err)
	// add profiling flag to enable profiling routes
	if viper.GetString("pprof-enabled") != "" {
		// pprof attaches routes to default serve mux
		// host:6061/debug/pprof/
		go func() {
			lg.Error().Err(http.ListenAndServe(":6061", http.DefaultServeMux))
		}()
	}

	// add our command line params to context
	ctx = context.WithValue(ctx, appctx.RatiosServerCTXKey, viper.Get("ratios-service"))
	ctx = context.WithValue(ctx, appctx.RatiosAccessTokenCTXKey, viper.Get("ratios-token"))
	ctx = context.WithValue(ctx, appctx.BaseCurrencyCTXKey, viper.Get("base-currency"))
	ctx = context.WithValue(ctx, appctx.RatiosCacheExpiryDurationCTXKey, viper.GetDuration("ratios-client-cache-expiry"))
	ctx = context.WithValue(ctx, appctx.RatiosCachePurgeDurationCTXKey, viper.GetDuration("ratios-client-cache-purge"))
	ctx = context.WithValue(ctx, appctx.DefaultACChoiceCTXKey, viper.GetFloat64("default-ac-choice"))
	ctx = context.WithValue(ctx, appctx.ParametersMergeBucketCTXKey, viper.Get("merge-param-bucket"))

	ctx = context.WithValue(ctx, appctx.ParametersVBATDeadlineCTXKey, viper.GetTime("vbat-deadline"))
	ctx = context.WithValue(ctx, appctx.ParametersTransitionCTXKey, viper.GetBool("transition"))

	var monthlyChoices []float64
	if err := viper.UnmarshalKey("default-monthly-choices", &monthlyChoices); err != nil {
		lg.Fatal().Err(err).Msg("failed to parse default-monthly-choices")
	}
	ctx = context.WithValue(ctx, appctx.DefaultMonthlyChoicesCTXKey, monthlyChoices)

	var tipChoices []float64
	if err := viper.UnmarshalKey("default-tip-choices", &tipChoices); err != nil {
		lg.Fatal().Err(err).Msg("failed to parse default-tip-choices")
	}
	ctx = context.WithValue(ctx, appctx.DefaultTipChoicesCTXKey, tipChoices)

	var acChoices []float64
	if err := viper.UnmarshalKey("default-ac-choices", &acChoices); err != nil {
		lg.Fatal().Err(err).Msg("failed to parse default-ac-choices")
	}
	if len(acChoices) > 0 {
		ctx = context.WithValue(ctx, appctx.DefaultACChoicesCTXKey, acChoices)
	}

	env := os.Getenv("ENV")
	if env == "" {
		lg.Fatal().Err(err).Msg("error retrieving environment")
	}

	tosVersion, err := strconv.Atoi(os.Getenv("REWARDS_TOS_VERSION"))
	if err != nil {
		lg.Fatal().Err(err).Msg("error retrieving rewards terms of service version")
	}

	// Get the bucket from the context and not os.Getenv so we don't diverge. GetParameters uses the context on
	// each request and this will need to be refactored before we can remove it.
	cardsBucket, ok := ctx.Value(appctx.ParametersMergeBucketCTXKey).(string)
	if !ok {
		lg.Fatal().Err(err).Msg("failed to get envar for cards bucket")
	}

	cardsKey := "cards.json"
	if ck := os.Getenv("CARDS-KEY"); ck != "" {
		cardsKey = ck
	}

	cfg := &rewards.Config{
		Env:        env,
		TOSVersion: tosVersion,
		Cards: &rewards.CardsConfig{
			Bucket: cardsBucket,
			Key:    cardsKey,
		},
	}

	s, err := rewards.InitService(ctx, cfg)
	if err != nil {
		lg.Fatal().Err(err).Msg("failed to initialize rewards service")
	}

	lg.Info().Str("service", fmt.Sprintf("%+v", s)).Msg("initialized service")

	r := cmd.SetupRouter(ctx)

	r.Get("/v1/parameters", middleware.InstrumentHandler("GetParametersHandler", rewards.GetParametersHandler(s)).ServeHTTP)

	r.Mount("/v1/cards", newCardsRouter(s))

	defer sentry.Flush(time.Second * 2)

	go func() {
		err := http.ListenAndServe(":9090", middleware.Metrics())
		if err != nil {
			sentry.CaptureException(err)
			lg.Panic().Err(err).Msg("metrics HTTP server start failed!")
		}
	}()

	srv := http.Server{
		Addr:         viper.GetString("address"),
		Handler:      chi.ServerBaseContext(ctx, r),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 20 * time.Second,
	}

	if err = srv.ListenAndServe(); err != nil {
		sentry.CaptureException(err)
		lg.Fatal().Err(err).Msg("HTTP server start failed!")
	}
}

func newCardsRouter(svc *rewards.Service) chi.Router {
	cardsRouter := chi.NewRouter()

	ch := handler.NewCardsHandler(svc)

	cardsRouter.Method(http.MethodGet, "/", middleware.InstrumentHandler("GetCards", handlers.AppHandler(ch.GetCardsHandler)))

	return cardsRouter
}

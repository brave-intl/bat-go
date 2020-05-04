package cmd

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/rewards"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
	"github.com/rs/zerolog/hlog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	baseCurrency string
)

func init() {
	// add complete and transform subcommand
	rewardsCmd.AddCommand(grpcCmd)
	rewardsCmd.AddCommand(restCmd)

	// add this command as a settlement subcommand
	serveCmd.AddCommand(rewardsCmd)

	// setup the flags

	// baseCurrency - defaults to USD
	rewardsCmd.PersistentFlags().StringVarP(&baseCurrency, "base-currency", "c", "USD",
		"the default base currency for the rewards system")
	must(viper.BindPFlag("base-currency", rewardsCmd.PersistentFlags().Lookup("base-currency")))
	must(viper.BindEnv("base-currency", "BASE_CURRENCY"))
}

func setupRouter(ctx context.Context) *chi.Mux {
	r := chi.NewRouter()
	r.Use(
		chiware.RequestID,
		chiware.RealIP,
		chiware.Heartbeat("/"),
		chiware.Timeout(10*time.Second),
		middleware.BearerToken,
		middleware.RateLimiter,
		middleware.RequestIDTransfer)
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger on context, make a new one
		_, logger = logging.SetupLogger(ctx)
	}
	if logger != nil {
		// Also handles panic recovery
		r.Use(
			hlog.NewHandler(*logger),
			hlog.UserAgentHandler("user_agent"),
			hlog.RequestIDHandler("req_id", "Request-Id"),
			middleware.RequestLogger(logger),
			chiware.Recoverer)
		log.Printf("server version/buildtime = %s %s %s", version, commit, buildTime)
	}
	// we will always have metrics and health-check
	r.Get("/metrics", middleware.Metrics())
	r.Get("/health-check", handlers.HealthCheckHandler(version, buildTime, commit))
	return r
}

var (
	rewardsCmd = &cobra.Command{
		Use:   "rewards",
		Short: "provides rewards micro-service entrypoint",
	}

	restCmd = &cobra.Command{
		Use:   "rest",
		Short: "provides REST api services",
		Run: func(cmd *cobra.Command, args []string) {
			logger, err := appctx.GetLogger(ctx)
			if err != nil {
				// no logger, setup
				ctx, logger = logging.SetupLogger(ctx)
			}

			// add our command line params to context
			ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, viper.Get("environment"))
			ctx = context.WithValue(ctx, appctx.RatiosServerCTXKey, viper.Get("ratios-service"))
			ctx = context.WithValue(ctx, appctx.RatiosAccessTokenCTXKey, viper.Get("ratios-token"))
			ctx = context.WithValue(ctx, appctx.BaseCurrencyCTXKey, viper.Get("base-currency"))

			// setup the service now
			s, err := rewards.InitService(ctx)
			if err != nil {
				logger.Error().Err(err).Msg("failed to initalize rewards service")
				os.Exit(1)
			}

			// do rest endpoints
			r := setupRouter(ctx)
			r.Get("/v1/parameters", middleware.InstrumentHandler(
				"GetParametersHandler", rewards.GetParametersHandler(s)).ServeHTTP)

			// setup server, and run
			srv := http.Server{
				Addr:         address,
				Handler:      chi.ServerBaseContext(ctx, r),
				ReadTimeout:  3 * time.Second,
				WriteTimeout: 15 * time.Second,
			}
			if err = srv.ListenAndServe(); err != nil {
				sentry.CaptureException(err)
				sentry.Flush(time.Second * 2)
				logger.Panic().Err(err).Msg("HTTP server start failed!")
			}

		},
	}

	grpcCmd = &cobra.Command{
		Use:   "grpc",
		Short: "provides gRPC api services",
		Run: func(cmd *cobra.Command, args []string) {
			// do gRPC
		},
	}
)

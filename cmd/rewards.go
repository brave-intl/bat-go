package cmd

import (
	"context"
	"time"

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/middleware"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
	"github.com/rs/zerolog/hlog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	defaultCurrency       string
	defaultTipChoices     string
	defaultMonthlyChoices string
)

func init() {
	// add complete and transform subcommand
	rewardsCmd.AddCommand(grpcCmd)
	rewardsCmd.AddCommand(restCmd)

	// add this command as a settlement subcommand
	serveCmd.AddCommand(rewardsCmd)

	// setup the flags

	// defaultCurrency - defaults to USD
	rewardsCmd.PersistentFlags().StringVarP(&defaultCurrency, "default-currency", "c", "USD",
		"the default base currency for the rewards system")
	must(viper.BindPFlag("default-currency", rewardsCmd.PersistentFlags().Lookup("default-currency")))
	must(viper.BindEnv("default-currency", "DEFAULT_CURRENCY"))

	// defaultTipChoices - defaults to USD
	rewardsCmd.PersistentFlags().StringVarP(&defaultTipChoices, "default-tip-choices", "", `1,10,100`,
		"the default tip choices for the rewards system")
	must(viper.BindPFlag("default-tip-choices", rewardsCmd.PersistentFlags().Lookup("default-tip-choices")))
	must(viper.BindEnv("default-tip-choices", "DEFAULT_TIP_CHOICES"))

	// defaultMonthlyChoices - defaults to USD
	rewardsCmd.PersistentFlags().StringVarP(&defaultMonthlyChoices, "default-monthly-choices", "", `1,10,100`,
		"the default monthly choices for the rewards system")
	must(viper.BindPFlag("default-monthly-choices", rewardsCmd.PersistentFlags().Lookup("default-monthly-choices")))
	must(viper.BindEnv("default-monthly-choices", "DEFAULT_MONTHLY_CHOICES"))
}

func setupRouter(ctx context.Context) *chi.Mux {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger on context, make a new one
		ctx, logger = logging.SetupLogger(ctx)
	}

	r := chi.NewRouter()
	r.Use(
		chiware.RequestID,
		chiware.RealIP,
		chiware.Heartbeat("/"),
		chiware.Timeout(10*time.Second),
		middleware.BearerToken,
		middleware.RateLimiter(ctx),
		middleware.RequestIDTransfer)
	if logger != nil {
		// Also handles panic recovery
		r.Use(
			hlog.NewHandler(*logger),
			hlog.UserAgentHandler("user_agent"),
			hlog.RequestIDHandler("req_id", "Request-Id"),
			middleware.RequestLogger(logger))

		logger.Info().
			Str("version", version).
			Str("commit", commit).
			Str("build_time", buildTime).
			Str("ratios_service", viper.GetString("ratios-service")).
			Str("address", viper.GetString("address")).
			Str("environment", viper.GetString("environment")).
			Msg("server starting")
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
		Run:   RewardsRestRun,
	}

	grpcCmd = &cobra.Command{
		Use:   "grpc",
		Short: "provides gRPC api services",
		Run:   RewardsGRPCRun,
	}
)

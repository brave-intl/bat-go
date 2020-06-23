package cmd

import (
	"context"
	"time"

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

func init() {
	rootCmd.AddCommand(serveCmd)

	// env - defaults to development
	serveCmd.PersistentFlags().StringVarP(&address, "address", "a", ":8080",
		"the default address to bind to")
	must(viper.BindPFlag("address", serveCmd.PersistentFlags().Lookup("address")))
	must(viper.BindEnv("address", "ADDR"))
}

var address string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "entrypoint to serve a micro-service",
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
			Str("ledger_service", viper.GetString("ledger-service")).
			Str("address", viper.GetString("address")).
			Str("environment", viper.GetString("environment")).
			Msg("server starting")
	}
	// we will always have metrics and health-check
	r.Get("/metrics", middleware.Metrics())
	r.Get("/health-check", handlers.HealthCheckHandler(version, buildTime, commit))
	return r
}

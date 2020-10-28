package cmd

import (
	"context"
	"time"

	"github.com/brave-intl/bat-go/middleware"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
	"github.com/rs/zerolog/hlog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	timeout = 10 * time.Second
)

func init() {
	RootCmd.AddCommand(ServeCmd)

	// address - sets the address of the server to be started
	ServeCmd.PersistentFlags().String("address", ":8080",
		"the default address to bind to")
	Must(viper.BindPFlag("address", ServeCmd.PersistentFlags().Lookup("address")))
	Must(viper.BindEnv("address", "ADDR"))
}

// ServeCmd the serve command
var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "entrypoint to serve a micro-service",
}

// SetupRouter sets up a router
func SetupRouter(ctx context.Context) *chi.Mux {
	logger, err := appctx.GetLogger(ctx)
	Must(err)

	r := chi.NewRouter()
	r.Use(
		chiware.RequestID,
		chiware.RealIP,
		chiware.Heartbeat("/"),
		chiware.Timeout(timeout),
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
			Str("version", ctx.Value(appctx.VersionCTXKey).(string)).
			Str("commit", ctx.Value(appctx.CommitCTXKey).(string)).
			Str("build_time", ctx.Value(appctx.BuildTimeCTXKey).(string)).
			Str("ratios_service", viper.GetString("ratios-service")).
			Str("address", viper.GetString("address")).
			Str("environment", viper.GetString("environment")).
			Msg("server starting")
	}
	// we will always have metrics and health-check
	r.Get("/metrics", middleware.Metrics())
	r.Get("/health-check", handlers.HealthCheckHandler(
		ctx.Value(appctx.VersionCTXKey).(string),
		ctx.Value(appctx.VersionCTXKey).(string),
		ctx.Value(appctx.VersionCTXKey).(string)))
	return r
}

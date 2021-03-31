package serve

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	// needed for profiling
	_ "net/http/pprof"
	// re-using viper bind-env for wallet env variables
	_ "github.com/brave-intl/bat-go/cmd/wallets"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/eyeshade"
	"github.com/brave-intl/bat-go/middleware"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	sentry "github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	// EyeshadeServerCmd start up the eyeshade server
	EyeshadeServerCmd = &cobra.Command{
		Use:   "eyeshade",
		Short: "subcommand to start up eyeshade server",
		Run:   cmd.Perform("eyeshade", RunEyeshadeServer),
	}
)

func init() {
	cmd.ServeCmd.AddCommand(EyeshadeServerCmd)
}

func setupEyeshadeMiddleware(ctx context.Context, logger *zerolog.Logger) *chi.Mux {
	buildTime := ctx.Value(appctx.BuildTimeCTXKey).(string)
	commit := ctx.Value(appctx.CommitCTXKey).(string)
	version := ctx.Value(appctx.VersionCTXKey).(string)

	logger.Info().
		Str("version", version).
		Str("commit", commit).
		Str("buildTime", buildTime).
		Msg("server starting up")

	govalidator.SetFieldsRequiredByDefault(true)

	r := chi.NewRouter()

	if os.Getenv("ENV") != "production" {
		r.Use(cors.Handler(cors.Options{
			Debug:            true,
			AllowedOrigins:   []string{"https://confab.bsg.brave.software", "https://together.bsg.brave.software"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "Digest", "Signature"},
			ExposedHeaders:   []string{"Link"},
			AllowCredentials: false,
			MaxAge:           300,
		}))
	}

	// chain should be:
	// id / transfer -> ip -> heartbeat -> request logger / recovery -> token check -> rate limit
	// -> instrumentation -> handler
	r.Use(chiware.RequestID)
	r.Use(middleware.RequestIDTransfer)

	// NOTE: This uses standard fowarding headers, note that this puts implicit trust in the header values
	// provided to us. In particular it uses the first element.
	// Consequently we should consider the request IP as primarily "informational".
	r.Use(chiware.RealIP)

	r.Use(chiware.Heartbeat("/"))
	// log and recover here
	if logger != nil {
		// Also handles panic recovery
		r.Use(hlog.NewHandler(*logger))
		r.Use(hlog.UserAgentHandler("user_agent"))
		r.Use(hlog.RequestIDHandler("req_id", "Request-Id"))
		r.Use(middleware.RequestLogger(logger))
	}
	// now we have middlewares we want included in logging
	r.Use(chiware.Timeout(15 * time.Second))
	r.Use(middleware.BearerToken)
	if os.Getenv("ENV") == "production" {
		r.Use(middleware.RateLimiter(ctx, 180))
	}
	// use cobra configurations for setting up eyeshade service
	// this way we can have the eyeshade service completely separated from
	// grants service and easily deployable.
	r.Get("/health-check", handlers.HealthCheckHandler(version, buildTime, commit))

	r.Get("/metrics", middleware.Metrics())

	// add profiling flag to enable profiling routes
	if os.Getenv("PPROF_ENABLED") != "" {
		// pprof attaches routes to default serve mux
		// host:6061/debug/pprof/
		go func() {
			log.Error().Err(http.ListenAndServe(":6061", http.DefaultServeMux))
		}()
	}

	return r
}

func setupEyeshadeRouters(
	ctx context.Context,
	logger *zerolog.Logger,
) (
	context.Context,
	*chi.Mux,
	*eyeshade.Service,
) {
	r := setupEyeshadeMiddleware(ctx, logger)

	s, err := eyeshade.SetupService(
		eyeshade.WithDBs,
		eyeshade.WithCommonClients,
		eyeshade.WithRouter,
	)
	if err != nil {
		sentry.CaptureException(err)
		logger.Panic().Err(err).Msg("unable to setup router")
	}
	r.Mount("/", s.Router())

	return ctx, r, s
}

// RunEyeshadeServer is the runner for starting up the eyeshade server
func RunEyeshadeServer(cmd *cobra.Command, args []string) error {
	enableJobWorkers, err := cmd.Flags().GetBool("enable-job-workers")
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	return EyeshadeServer(
		ctx,
		enableJobWorkers,
	)
}

// EyeshadeServer runs the eyeshade server
func EyeshadeServer(
	ctx context.Context,
	enableJobWorkers bool,
) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}

	sentryDsn := os.Getenv("SENTRY_DSN")
	if sentryDsn != "" {
		buildTime := ctx.Value(appctx.BuildTimeCTXKey).(string)
		commit := ctx.Value(appctx.CommitCTXKey).(string)
		err := sentry.Init(sentry.ClientOptions{
			Dsn:     sentryDsn,
			Release: fmt.Sprintf("bat-go@%s-%s", commit, buildTime),
		})
		defer sentry.Flush(2 * time.Second)
		if err != nil {
			logger.Panic().Err(err).Msg("unable to setup reporting!")
		}
	}
	logger.Info().
		Str("prefix", "main").
		Msg("Starting server")

	ctx, r, _ := setupEyeshadeRouters(ctx, logger)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		err := http.ListenAndServe(":9090", middleware.Metrics())
		if err != nil {
			sentry.CaptureException(err)
			logger.Panic().Err(err).Msg("metrics HTTP server start failed!")
		}
	}()

	srv := http.Server{
		Addr:         ":3333",
		Handler:      chi.ServerBaseContext(ctx, r),
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 20 * time.Second,
	}
	err = srv.ListenAndServe()
	if err != nil {
		sentry.CaptureException(err)
		return errorutils.Wrap(err, "HTTP server start failed!")
	}
	return nil
}

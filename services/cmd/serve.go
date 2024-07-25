package cmd

import (
	"context"
	"os"
	"time"

	cmdutils "github.com/brave-intl/payments-service/cmd"
	rootcmd "github.com/brave-intl/payments-service/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/middleware"
	srv "github.com/brave-intl/bat-go/libs/service"
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
	rootcmd.RootCmd.AddCommand(ServeCmd)

	// address - sets the address of the server to be started
	ServeCmd.PersistentFlags().String("address", ":8080",
		"the default address to bind to")
	cmdutils.Must(viper.BindPFlag("address", ServeCmd.PersistentFlags().Lookup("address")))
	cmdutils.Must(viper.BindEnv("address", "ADDR"))

	ServeCmd.PersistentFlags().Bool("enable-job-workers", true,
		"enable job workers (defaults true)")
	cmdutils.Must(viper.BindPFlag("enable-job-workers", ServeCmd.PersistentFlags().Lookup("enable-job-workers")))
	cmdutils.Must(viper.BindEnv("enable-job-workers", "ENABLE_JOB_WORKERS"))
}

// ServeCmd the serve command
var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "entrypoint to serve a micro-service",
}

// SetupRouter sets up a router
func SetupRouter(ctx context.Context) *chi.Mux {
	logger, err := appctx.GetLogger(ctx)
	cmdutils.Must(err)

	r := chi.NewRouter()
	r.Use(
		chiware.RequestID,
		chiware.RealIP,
		chiware.Heartbeat("/"),
		chiware.Timeout(timeout),
		middleware.BearerToken,

		middleware.RequestIDTransfer)

	if os.Getenv("ENV") == "production" {
		rl, ok := ctx.Value(appctx.RateLimitPerMinuteCTXKey).(int)
		if !ok {
			r.Use(middleware.RateLimiter(ctx, 180))
		} else {
			r.Use(middleware.RateLimiter(ctx, rl))
		}
	}
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
	r.Get("/health-check", handlers.HealthCheckHandler(
		ctx.Value(appctx.VersionCTXKey).(string),
		ctx.Value(appctx.VersionCTXKey).(string),
		ctx.Value(appctx.VersionCTXKey).(string), nil, nil))
	return r
}

// SetupJobWorkers - setup job workers
func SetupJobWorkers(ctx context.Context, jobs []srv.Job) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}

	enableJobWorkers, err := ServeCmd.Flags().GetBool("enable-job-workers")
	if err != nil {
		return err
	}

	if enableJobWorkers {
		for _, job := range jobs {
			// iterate over jobs
			for i := 0; i < job.Workers; i++ {
				// spin up a job worker for each worker
				logger.Debug().Msg("starting job worker")
				go srv.JobWorker(ctx, job.Func, job.Cadence)
			}
		}
	}
	return nil
}

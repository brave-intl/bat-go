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
	"github.com/brave-intl/bat-go/eyeshade"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/middleware"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/logging"
	sentry "github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
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

// WithService creates a service
func WithService(
	ctx context.Context,
	_ *eyeshade.Service,
) (*eyeshade.Service, error) {
	return eyeshade.SetupService(
		eyeshade.WithContext(ctx),
		eyeshade.WithNewLogger,
		eyeshade.WithBuildInfo,
		eyeshade.WithNewDBs,
		eyeshade.WithNewClients,
		eyeshade.WithNewRouter,
		eyeshade.WithMiddleware,
		eyeshade.WithRoutes,
	)
}

// RunEyeshadeServer is the runner for starting up the eyeshade server
func RunEyeshadeServer(cmd *cobra.Command, args []string) error {
	// enableJobWorkers, err := cmd.Flags().GetBool("enable-job-workers")
	// if err != nil {
	// 	return err
	// }
	ctx := cmd.Context()
	err := EyeshadeServer(
		ctx,
		true,
		WithService,
	)
	if err == nil {
		return nil
	}
	sentry.CaptureException(err)
	return errorutils.Wrap(err, "HTTP server start failed!")
}

// EyeshadeServer runs the eyeshade server
func EyeshadeServer(
	ctx context.Context,
	enableJobWorkers bool,
	params ...func(context.Context, *eyeshade.Service) (*eyeshade.Service, error),
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

	var service *eyeshade.Service
	for _, setup := range params {
		service, err = setup(ctx, service)
		if err != nil {
			return err
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		err := http.ListenAndServe(":9090", middleware.Metrics())
		if err != nil {
			sentry.CaptureException(err)
			logger.Panic().Err(err).Msg("metrics HTTP server start failed!")
		}
	}()

	addr := ":3333"
	srv := http.Server{
		Addr:         addr,
		Handler:      chi.ServerBaseContext(ctx, service.Router()),
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 20 * time.Second,
	}
	logger.Info().
		Str("prefix", "main").
		Str("addr", addr).
		Msg("Starting server")

	err = srv.ListenAndServe()
	if err != nil {
		return errorutils.Wrap(err, "HTTP server start failed!")
	}
	return nil
}

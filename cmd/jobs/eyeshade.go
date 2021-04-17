package jobs

import (
	"context"
	"fmt"
	"os"
	"time"

	// needed for profiling
	_ "net/http/pprof"
	// re-using viper bind-env for wallet env variables
	_ "github.com/brave-intl/bat-go/cmd/wallets"
	eyeshade "github.com/brave-intl/bat-go/eyeshade/service"

	"github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/logging"
	sentry "github.com/getsentry/sentry-go"
	"github.com/spf13/cobra"
)

var (
	// EyeshadeSurveyorFreezeCmd start up the eyeshade server
	EyeshadeSurveyorFreezeCmd = &cobra.Command{
		Use:   "eyeshade",
		Short: "subcommand to start freezing eyeshade surveyors",
		Run:   cmd.Perform("eyeshade", RunEyeshadeSurveyorFreezeCmd),
	}
)

func init() {
	cmd.ServeCmd.AddCommand(EyeshadeSurveyorFreezeCmd)
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
	)
}

// RunEyeshadeSurveyorFreezeCmd is the runner for starting up the eyeshade server
func RunEyeshadeSurveyorFreezeCmd(cmd *cobra.Command, args []string) error {
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

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var service *eyeshade.Service
	for _, param := range params {
		service, err = param(ctx, service)
		if err != nil {
			return err
		}
	}
	// freeze surveyors, looking back 1 day by default
	return service.FreezeSurveyors()
}

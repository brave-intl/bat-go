package submit

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	loggingutils "github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/settlement/cmd/settlement"
	"github.com/brave-intl/bat-go/services/settlement/submit/internal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// WorkerCmd starts submit worker
	WorkerCmd = &cobra.Command{
		Short: "starts settlement submit worker",
		Use:   "submit-worker",
		Run:   StartSubmitWorker,
	}
)

func init() {
	settlement.Cmd.AddCommand(WorkerCmd)
}

// StartSubmitWorker initializes and starts a new instance of submit worker.
func StartSubmitWorker(command *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(command.Context())
	logger := loggingutils.Logger(ctx, "SubmitWorker")

	config, err := internal.NewSubmitConfig(
		internal.WithRedisAddress(viper.GetString("REDIS_ADDRESS")),
		internal.WithRedisUsername(viper.GetString("REDIS_USERNAME")),
		internal.WithRedisPassword(viper.GetString("REDIS_PASSWORD")),
		internal.WithPaymentClient(viper.GetString("PAYMENT_SERVICE_URL")))
	if err != nil {
		logger.Fatal().Err(err).
			Msg("error creating submit config")
	}

	logger.Info().Msg("starting submit worker")
	worker := internal.CreateSubmitWorker(config)

	go worker.Run(ctx)

	logger.Info().Msg("submit worker started")

	//TODO make graceful shutdown

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown

	cancel()

	logger.Info().Msg("shutting down submit worker")

	close(shutdown)
}

package prepare

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	loggingutils "github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/settlement/cmd/settlement"
	"github.com/brave-intl/bat-go/services/settlement/prepare/internal/prepare"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// WorkerCmd starts prepare worker
	WorkerCmd = &cobra.Command{
		Short: "starts settlement prepare worker",
		Use:   "prepare-worker",
		Run:   StartPrepareWorker,
	}
)

func init() {
	settlement.Cmd.AddCommand(WorkerCmd)
}

// StartPrepareWorker initializes and starts a new instance of prepare worker.
func StartPrepareWorker(command *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(command.Context())
	logger := loggingutils.Logger(ctx, "Worker")

	config, err := prepare.NewWorkerConfig(
		prepare.WithRedisAddress(viper.GetString("REDIS_ADDRESS")),
		prepare.WithRedisUsername(viper.GetString("REDIS_USERNAME")),
		prepare.WithRedisPassword(viper.GetString("REDIS_PASSWORD")),
		prepare.WithPaymentClient(viper.GetString("PAYMENT_SERVICE_URL")),
		prepare.WithReportBucket(viper.GetString("SETTLEMENTS_TXN_BUCKET")),
		prepare.WithNotificationTopic("TODO"))
	if err != nil {
		logger.Fatal().Err(err).
			Msg("error creating prepare config")
	}

	logger.Info().Msg("starting prepare worker")

	worker, err := prepare.CreateWorker(ctx, config)
	if err != nil {
		logger.Fatal().Err(err).
			Msg("error creating prepare worker")
	}

	go worker.Run(ctx)

	logger.Info().Msg("prepare worker started")

	//TODO make graceful shutdown

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown

	cancel()

	logger.Info().Msg("shutting down prepare worker")

	close(shutdown)
}

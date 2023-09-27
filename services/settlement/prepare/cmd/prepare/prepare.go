package prepare

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/brave-intl/bat-go/libs/logging"
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
func StartPrepareWorker(command *cobra.Command, _ []string) {
	ctx, cancel := context.WithCancel(command.Context())
	l := logging.Logger(ctx, "worker")

	conf, err := prepare.NewWorkerConfig(
		prepare.WithRedisAddress(viper.GetString("REDIS_ADDRESS")),
		prepare.WithRedisUsername(viper.GetString("REDIS_USERNAME")),
		prepare.WithRedisPassword(viper.GetString("REDIS_PASSWORD")),
		prepare.WithPaymentClient(viper.GetString("PAYMENT_SERVICE_URL")),
		prepare.WithReportBucket(viper.GetString("SETTLEMENTS_TXN_BUCKET")),
		prepare.WithNotificationTopic("TODO"))
	if err != nil {
		l.Fatal().Err(err).Msg("error creating prepare config")
	}

	l.Info().Msg("starting prepare worker")

	worker, err := prepare.CreateWorker(ctx, conf)
	if err != nil {
		l.Fatal().Err(err).Msg("error creating prepare worker")
	}

	go worker.Run(ctx)

	l.Info().Msg("prepare worker started")

	//TODO make graceful shutdown

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown

	cancel()

	l.Info().Msg("shutting down prepare worker")

	close(shutdown)
}

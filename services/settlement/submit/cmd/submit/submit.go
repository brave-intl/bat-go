package submit

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/settlement/cmd/settlement"
	"github.com/brave-intl/bat-go/services/settlement/submit/internal/submit"
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
func StartSubmitWorker(command *cobra.Command, _ []string) {
	ctx, cancel := context.WithCancel(command.Context())
	l := logging.Logger(ctx, "worker")

	config, err := submit.NewWorkerConfig(
		submit.WithRedisAddress(viper.GetString("REDIS_ADDRESS")),
		submit.WithRedisUsername(viper.GetString("REDIS_USERNAME")),
		submit.WithRedisPassword(viper.GetString("REDIS_PASSWORD")),
		submit.WithPaymentClient(viper.GetString("PAYMENT_SERVICE_URL")))
	if err != nil {
		l.Fatal().Err(err).Msg("error creating submit config")
	}

	l.Info().Msg("starting submit worker")

	worker, err := submit.CreateWorker(config)
	if err != nil {
		l.Fatal().Err(err).Msg("error creating submit worker")
	}

	go worker.Run(ctx)

	l.Info().Msg("submit worker started")

	//TODO make graceful shutdown

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown

	cancel()

	l.Info().Msg("shutting down submit worker")

	close(shutdown)
}

package submit

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	appctx "github.com/brave-intl/bat-go/libs/context"
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

// StartSubmitWorker initializes and starts submit worker.
func StartSubmitWorker(command *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(command.Context())
	ctx = context.WithValue(ctx, appctx.SettlementRedisAddressCTXKey, viper.Get("REDIS_ADDRESS"))
	ctx = context.WithValue(ctx, appctx.SettlementRedisUsernameCTXKey, viper.Get("REDIS_USERNAME"))
	ctx = context.WithValue(ctx, appctx.SettlementRedisPasswordCTXKey, viper.Get("REDIS_PASSWORD"))
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, viper.Get("PAYMENT_SERVICE_URL"))

	logger := loggingutils.Logger(ctx, "SubmitWorker")
	logger.Info().Msg("starting submit worker")

	s, err := internal.NewSubmitWorker(ctx)
	if err != nil {
		logger.Fatal().Err(err).
			Msg("error creating submit worker")
	}

	go s.Run(ctx)

	logger.Info().Msg("submit worker started")

	//TODO make graceful shutdown

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown

	cancel()

	logger.Info().Msg("shutting down submit worker")
}

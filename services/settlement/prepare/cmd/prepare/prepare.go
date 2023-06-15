package prepare

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/brave-intl/bat-go/services/settlement/prepare/internal"

	appctx "github.com/brave-intl/bat-go/libs/context"
	loggingutils "github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/settlement/cmd/settlement"
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

// StartPrepareWorker initializes and starts prepare worker
func StartPrepareWorker(command *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(command.Context())
	ctx = context.WithValue(ctx, appctx.SettlementRedisAddressCTXKey, viper.Get("REDIS_ADDRESS"))
	ctx = context.WithValue(ctx, appctx.SettlementRedisUsernameCTXKey, viper.Get("REDIS_USERNAME"))
	ctx = context.WithValue(ctx, appctx.SettlementRedisPasswordCTXKey, viper.Get("REDIS_PASSWORD"))
	ctx = context.WithValue(ctx, appctx.SettlementPayoutReportBucketCTXKey, viper.Get("SETTLEMENTS_TXN_BUCKET"))
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, viper.Get("PAYMENT_SERVICE_URL"))

	logger := loggingutils.Logger(ctx, "PrepareWorker")
	logger.Info().Msg("starting prepare worker")

	p, err := internal.NewPrepareWorker(ctx)
	if err != nil {
		logger.Fatal().Err(err).
			Msg("error creating prepare worker")
	}

	go p.Run(ctx)

	logger.Info().Msg("prepare worker started")

	//TODO make graceful shutdown

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown

	cancel()

	logger.Info().Msg("shutting down prepare worker")

	close(shutdown)
}

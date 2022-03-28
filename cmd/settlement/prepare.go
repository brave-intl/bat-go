package settlement

import (
	"context"
	"github.com/brave-intl/bat-go/settlement/automation/prepare"
	"os"
	"os/signal"
	"syscall"

	appctx "github.com/brave-intl/bat-go/utils/context"
	loggingutils "github.com/brave-intl/bat-go/utils/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// PrepareWorkerCmd starts prepare worker
	PrepareWorkerCmd = &cobra.Command{
		Short: "starts settlement prepare worker",
		Use:   "prepare-worker",
		Run:   StartPrepareWorker,
	}
)

func init() {
	SettlementCmd.AddCommand(PrepareWorkerCmd)
}

// StartPrepareWorker initializes and starts the prepare worker
func StartPrepareWorker(command *cobra.Command, args []string) {
	ctx := command.Context()
	ctx = context.WithValue(ctx, appctx.RedisSettlementURLCTXKey, viper.Get(""))
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, viper.Get(""))

	loggingutils.FromContext(ctx).Info().Msg("starting prepare consumer")

	err := prepare.StartConsumer(ctx)
	if err != nil {
		loggingutils.FromContext(ctx).Error().Err(err).Msg("error starting prepare consumer")
		return
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown

	loggingutils.FromContext(ctx).Info().Msg("shutting down prepare consumer")

	close(shutdown)
}

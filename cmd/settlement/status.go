package settlement

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/brave-intl/bat-go/settlement/automation/status"

	appctx "github.com/brave-intl/bat-go/utils/context"
	loggingutils "github.com/brave-intl/bat-go/utils/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// start check status worker
	CheckStatusWorkerCmd = &cobra.Command{
		Short: "starts settlement check status worker",
		Use:   "check-status-worker",
		Run:   StartCheckStatusWorker,
	}
)

func init() {
	SettlementCmd.AddCommand(CheckStatusWorkerCmd)
}

// StartCheckStatusWorker initializes and starts the check status worker
func StartCheckStatusWorker(command *cobra.Command, args []string) {
	ctx := command.Context()
	ctx = context.WithValue(ctx, appctx.RedisSettlementURLCTXKey, viper.Get(""))
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, viper.Get(""))

	loggingutils.FromContext(ctx).Info().Msg("starting check status worker")

	err := status.StartConsumer(ctx)
	if err != nil {
		loggingutils.FromContext(ctx).Error().Err(err).Msg("error starting consumer")
		return
	}

	shutdown := make(chan os.Signal)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown

	loggingutils.FromContext(ctx).Info().Msg("shutting down check status worker")

	close(shutdown)
}

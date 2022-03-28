package settlement

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/brave-intl/bat-go/settlement/automation/submit"

	appctx "github.com/brave-intl/bat-go/utils/context"
	loggingutils "github.com/brave-intl/bat-go/utils/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// SubmitWorkerCmd starts submit worker
	SubmitWorkerCmd = &cobra.Command{
		Short: "starts settlement submit worker",
		Use:   "submit-worker",
		Run:   StartSubmitWorker,
	}
)

func init() {
	SettlementCmd.AddCommand(SubmitWorkerCmd)
}

// StartSubmitWorker initializes and starts the submit worker
func StartSubmitWorker(command *cobra.Command, args []string) {
	ctx := command.Context()
	ctx = context.WithValue(ctx, appctx.RedisSettlementURLCTXKey, viper.Get(""))
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, viper.Get(""))

	loggingutils.FromContext(ctx).Info().Msg("starting submit worker")

	err := submit.StartConsumer(ctx)
	if err != nil {
		loggingutils.FromContext(ctx).Error().Err(err).Msg("error starting consumer")
		return
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown

	loggingutils.FromContext(ctx).Info().Msg("shutting down submit worker")

	close(shutdown)
}

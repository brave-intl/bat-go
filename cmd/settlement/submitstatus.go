package settlement

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/brave-intl/bat-go/settlement/automation/submitstatus"
	appctx "github.com/brave-intl/bat-go/utils/context"
	loggingutils "github.com/brave-intl/bat-go/utils/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// SubmitStatusWorkerCmd starts the submit status worker.
	SubmitStatusWorkerCmd = &cobra.Command{
		Short: "starts settlement submit status worker",
		Use:   "submit-status-worker",
		Run:   StartSubmitStatusWorker,
	}
)

func init() {
	SettlementCmd.AddCommand(SubmitStatusWorkerCmd)
}

// StartSubmitStatusWorker initializes and starts submit status worker.
func StartSubmitStatusWorker(command *cobra.Command, args []string) {
	ctx := command.Context()
	ctx = context.WithValue(ctx, appctx.SettlementRedisAddressCTXKey, viper.Get("REDIS_ADDRESS"))
	ctx = context.WithValue(ctx, appctx.SettlementRedisPasswordCTXKey, viper.Get("REDIS_USERNAME"))
	ctx = context.WithValue(ctx, appctx.SettlementRedisPasswordCTXKey, viper.Get("REDIS_PASSWORD"))
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, viper.Get("PAYMENT_SERVICE_URL"))
	ctx = context.WithValue(ctx, appctx.PaymentServiceHTTPSingingKeyHexCTXKey, viper.Get("PAYMENT_SERVICE_SIGNATOR_PRIVATE_KEY_HEX"))

	loggingutils.FromContext(ctx).Info().Msg("starting submit status worker")

	err := submitstatus.StartConsumer(ctx)
	if err != nil {
		loggingutils.FromContext(ctx).Error().Err(err).Msg("error starting consumer")
		return
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown

	loggingutils.FromContext(ctx).Info().Msg("shutting down submit status worker")

	close(shutdown)
}

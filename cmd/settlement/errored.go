package settlement

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/brave-intl/bat-go/settlement/automation/notify"
	appctx "github.com/brave-intl/bat-go/utils/context"
	loggingutils "github.com/brave-intl/bat-go/utils/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// ErroredWorkerCmd starts errored worker
	ErroredWorkerCmd = &cobra.Command{
		Short: "starts settlement errored worker",
		Use:   "errored-worker",
		Run:   StartErroredWorker,
	}
)

func init() {
	SettlementCmd.AddCommand(ErroredWorkerCmd)
}

// StartErroredWorker initializes and starts errored worker
func StartErroredWorker(command *cobra.Command, args []string) {
	ctx := command.Context()
	ctx = context.WithValue(ctx, appctx.SettlementRedisAddressCTXKey, viper.Get("REDIS_ADDRESS"))
	ctx = context.WithValue(ctx, appctx.SettlementRedisUsernameCTXKey, viper.Get("REDIS_USERNAME"))
	ctx = context.WithValue(ctx, appctx.SettlementRedisPasswordCTXKey, viper.Get("REDIS_PASSWORD"))
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, viper.Get("PAYMENT_SERVICE_URL"))
	ctx = context.WithValue(ctx, appctx.PaymentServiceHTTPSingingKeyHexCTXKey, viper.Get("PAYMENT_SERVICE_SIGNATOR_PRIVATE_KEY_HEX"))

	loggingutils.FromContext(ctx).Info().Msg("starting errored consumer")

	err := notify.StartConsumer(ctx)
	if err != nil {
		loggingutils.FromContext(ctx).Error().Err(err).Msg("error starting errored consumer")
		return
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown

	loggingutils.FromContext(ctx).Info().Msg("shutting down errored consumer")

	close(shutdown)
}

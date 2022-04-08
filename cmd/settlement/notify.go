package settlement

import (
	"context"
	"github.com/brave-intl/bat-go/settlement/automation/notify"
	appctx "github.com/brave-intl/bat-go/utils/context"
	loggingutils "github.com/brave-intl/bat-go/utils/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"os/signal"
	"syscall"
)

var (
	// NotifyWorkerCmd starts notify worker
	NotifyWorkerCmd = &cobra.Command{
		Short: "starts settlement notify worker",
		Use:   "notify-worker",
		Run:   StartNotifyWorker,
	}
)

func init() {
	SettlementCmd.AddCommand(PrepareWorkerCmd)
}

// StartNotifyWorker initializes and starts notify worker
func StartNotifyWorker(command *cobra.Command, args []string) {
	ctx := command.Context()
	ctx = context.WithValue(ctx, appctx.RedisSettlementURLCTXKey, viper.Get("REDIS_ADDRESS"))
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, viper.Get("PAYMENT_SERVICE_URL"))
	ctx = context.WithValue(ctx, appctx.PaymentServiceHTTPSingingKeyCTXKey, viper.Get("PAYMENT_SERVICE_HTTP_SIGN_KEY"))

	loggingutils.FromContext(ctx).Info().Msg("starting notify consumer")

	err := notify.StartConsumer(ctx)
	if err != nil {
		loggingutils.FromContext(ctx).Error().Err(err).Msg("error starting notify consumer")
		return
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown

	loggingutils.FromContext(ctx).Info().Msg("shutting down notify consumer")

	close(shutdown)
}

package consumers

import (
	"context"
	"time"

	// needed for profiling
	_ "net/http/pprof"
	// re-using viper bind-env for wallet env variables
	_ "github.com/brave-intl/bat-go/cmd/wallets"
	"github.com/brave-intl/bat-go/eyeshade/avro"
	eyeshade "github.com/brave-intl/bat-go/eyeshade/service"

	"github.com/brave-intl/bat-go/cmd"
	sentry "github.com/getsentry/sentry-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// EyeshadeConsumersCmd start up the eyeshade server
	EyeshadeConsumersCmd = &cobra.Command{
		Use:   "eyeshade",
		Short: "subcommand to start eyeshade consumers",
		Run:   cmd.Perform("eyeshade", RunEyeshadeConsumersCmd),
	}
)

func init() {
	ConsumersCmd.AddCommand(EyeshadeConsumersCmd)
	eyeshadeConsumersFlags := cmd.NewFlagBuilder(EyeshadeConsumersCmd)
	eyeshadeConsumersFlags.Flag().Int("max-batch-size", 100,
		"the maximum number of messages that a batch should consume at one time").
		Env("KAFKA_MAX_BATCH_SIZE").
		Bind("max-batch-size")
}

// WithService creates a service
func WithService(
	ctx context.Context,
) (*eyeshade.Service, error) {
	batchSize := viper.GetViper().GetInt("max-batch-size")
	return eyeshade.SetupService(
		eyeshade.WithContext(ctx),
		eyeshade.WithNewLogger,
		eyeshade.WithBuildInfo,
		eyeshade.WithNewDBs,
		eyeshade.WithConsumer(batchSize, avro.AllTopics...),
		eyeshade.WithTopicAutoCreation,
	)
}

// RunEyeshadeConsumersCmd is the runner for starting up the eyeshade server
func RunEyeshadeConsumersCmd(cmd *cobra.Command, args []string) error {
	service, err := WithService(
		cmd.Context(),
	)
	if err == nil {
		return nil
	}
	errCh := service.Consume()
	for {
		select {
		case err := <-errCh:
			sentry.CaptureException(err)
		case <-time.After(time.Second * 10):
			// get rid of warning about for loop select combination
		}
	}
}

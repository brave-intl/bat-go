package cmd

import (
	"fmt"

	rootcmd "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/services/payments"
	redis "github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
)

// WorkerRun is the endpoint for running the payment worker
func WorkerRun(command *cobra.Command, args []string) {
	ctx := command.Context()
	logger, err := appctx.GetLogger(ctx)
	rootcmd.Must(err)

	// FIXME
	redisClient := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", "127.0.0.1", "6379"),
	})

	worker := payments.NewWorker(redisClient)

	go func() {
		err = worker.StartPrepareConfigConsumer(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("consumer exited with error")
		}
	}()

	err = worker.StartSubmitConfigConsumer(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("consumer exited with error")
	}

}

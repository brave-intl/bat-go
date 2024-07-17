package cmd

import (
	rootcmd "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/nitro"
	"github.com/brave-intl/bat-go/libs/redisconsumer"
	"github.com/brave-intl/bat-go/services/payments"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// WorkerRun is the endpoint for running the payment worker
func WorkerRun(command *cobra.Command, args []string) {
	ctx := command.Context()
	logger, err := appctx.GetLogger(ctx)
	rootcmd.Must(err)

	env := viper.GetString("environment")
	addr := viper.GetString("redis-addr")
	user := viper.GetString("redis-user")
	pass := viper.GetString("redis-pass")

	redisUseTLS := true
	if nitro.EnclaveMocking() {
		redisUseTLS = false
	}
	redisClient, err := redisconsumer.NewStreamClient(ctx, env, addr, user, pass, redisUseTLS)
	if err != nil {
		logger.Error().Err(err).Msg("failed to start redis consumer")
		return
	}

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

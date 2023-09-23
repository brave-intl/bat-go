package cmd

import (
	"fmt"

	rootcmd "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/services/payments"
	redis "github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
)

// WorkerRun - FIXME
func WorkerRun(command *cobra.Command, args []string) {
	ctx := command.Context()
	logger, err := appctx.GetLogger(ctx)
	rootcmd.Must(err)

	// FIXME
	stream := "prepare-config"
	consumerGroup := "prepare-config-cg"
	redisClient := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", "127.0.0.1", "6379"),
	})

	service := payments.NewRedisService(redisClient)

	err = payments.NewConsumer(ctx, redisClient, stream, consumerGroup, "0", service.HandlePrepareConfigMessage)
	if err != nil {
		logger.Error().Err(err).Msg("consumer exited with error")
	}
}

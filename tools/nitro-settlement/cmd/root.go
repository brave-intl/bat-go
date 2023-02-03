package cmd

import (
	"context"
	"os"

	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// rootCmd is the base command (what the binary is called)
	rootCmd = &cobra.Command{
		Use:   "settlement-cli",
		Short: "settlement-cli is the tooling for nitro settlement operators",
	}
	redisAddrKey = "redis-addrs"
	redisUserKey = "redis-user"
	redisPassKey = "redis-pass"
	testModeKey  = "test-mode"
)

func init() {
	// global persistent flags

	// redis addr/pass
	rootCmd.PersistentFlags().StringSlice(redisAddrKey, nil, "the redis host for streams (redis:1234)")
	viper.BindPFlag(redisAddrKey, rootCmd.PersistentFlags().Lookup(redisAddrKey))

	rootCmd.PersistentFlags().String(redisUserKey, "", "the redis username")
	viper.BindPFlag(redisUserKey, rootCmd.PersistentFlags().Lookup(redisUserKey))

	rootCmd.PersistentFlags().String(redisPassKey, "", "the redis password")
	viper.BindPFlag(redisPassKey, rootCmd.PersistentFlags().Lookup(redisPassKey))
	viper.BindEnv(redisPassKey, "REDIS_PASS")

	rootCmd.PersistentFlags().Bool(testModeKey, false, "toggle test mode")
	viper.BindPFlag(testModeKey, rootCmd.PersistentFlags().Lookup(testModeKey))
}

// Execute - the main entrypoint for all subcommands in settlement-cli
func Execute(ctx context.Context) {
	// execute the command
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		logging.Logger(ctx, "Execute").Error().Err(err).Msg("./settlement-cli command encountered an error")
		os.Exit(1)
	}
}

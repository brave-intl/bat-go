package cmd

import (
	"context"
	"log"
	"os"
	"time"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// RootCmd is the base command (what the binary is called)
	RootCmd = &cobra.Command{
		Use:   "bat-go",
		Short: "bat-go provides go based services and processes for BAT",
	}
	ctx = context.Background()

	// variables will be overwritten at build time
	version   string
	commit    string
	buildTime string

	// top level config items
	pprofEnabled string
	env          string

	ratiosClientPurge  time.Duration
	ratiosClientExpiry time.Duration
)

// Must helper to make sure there is no errors
func Must(err error) {
	if err != nil {
		log.Printf("failed to initialize: %s\n", err.Error())
		// exit with failure
		os.Exit(1)
	}
}

// Execute - the main entrypoint for all subcommands in bat-go
func Execute() {
	// setup context with logging, but first we need to setup the environment
	var logger *zerolog.Logger
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, viper.Get("environment"))
	ctx, logger = logging.SetupLogger(ctx)
	// setup ratios service values
	ctx = context.WithValue(ctx, appctx.RatiosServerCTXKey, viper.Get("ratios-service"))
	ctx = context.WithValue(ctx, appctx.RatiosAccessTokenCTXKey, viper.Get("ratios-token"))

	// execute the root cmd
	if err := RootCmd.ExecuteContext(ctx); err != nil {
		logger.Error().Err(err).Msg("./bat-go command encountered an error")
		os.Exit(1)
	}
}

func init() {

	// pprof-enabled - defaults to ""
	RootCmd.PersistentFlags().StringVarP(&pprofEnabled, "pprof-enabled", "", "",
		"pprof enablement")
	Must(viper.BindPFlag("pprof-enabled", RootCmd.PersistentFlags().Lookup("pprof-enabled")))
	Must(viper.BindEnv("pprof-enabled", "PPROF_ENABLED"))

	// env - defaults to local
	RootCmd.PersistentFlags().StringVarP(&env, "environment", "e", "local",
		"the default environment")
	Must(viper.BindPFlag("environment", RootCmd.PersistentFlags().Lookup("environment")))
	Must(viper.BindEnv("environment", "ENV"))

	// ratiosAccessToken (required by all)
	RootCmd.PersistentFlags().String("ratios-token", "",
		"the ratios service token for this service")
	Must(viper.BindPFlag("ratios-token", RootCmd.PersistentFlags().Lookup("ratios-token")))
	Must(viper.BindEnv("ratios-token", "RATIOS_TOKEN"))

	// ratiosService (required by all)
	RootCmd.PersistentFlags().StringP("ratios-service", "r", "",
		"the ratios service address")
	Must(viper.BindPFlag("ratios-service", RootCmd.PersistentFlags().Lookup("ratios-service")))
	Must(viper.BindEnv("ratios-service", "RATIOS_SERVICE"))

	// ratiosClientExpiry
	RootCmd.PersistentFlags().DurationVarP(&ratiosClientExpiry, "ratios-client-cache-expiry", "", 5*time.Second,
		"the ratios client cache default eviction duration")
	Must(viper.BindPFlag("ratios-client-cache-expiry", RootCmd.PersistentFlags().Lookup("ratios-client-cache-expiry")))
	Must(viper.BindEnv("ratios-client-cache-expiry", "RATIOS_CACHE_EXPIRY"))

	// ratiosClientPurge
	RootCmd.PersistentFlags().DurationVarP(&ratiosClientPurge, "ratios-client-cache-purge", "", 1*time.Minute,
		"the ratios client cache default purge duration")
	Must(viper.BindPFlag("ratios-client-cache-purge", RootCmd.PersistentFlags().Lookup("ratios-client-cache-purge")))
	Must(viper.BindEnv("ratios-client-cache-purge", "RATIOS_CACHE_PURGE"))
}

// Perform performs a run
func Perform(action string, fn func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		if err := fn(cmd, args); err != nil {
			log.Printf("failed to %s: %s\n", action, err)
			os.Exit(1)
		}
	}
}

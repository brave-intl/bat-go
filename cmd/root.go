package cmd

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	rootCmd = &cobra.Command{
		Use:   "bat-go",
		Short: "bat-go provides go based services and processes for BAT",
	}
	ctx = context.Background()

	// variables will be overwritten at build time
	version   string
	commit    string
	buildTime string

	// top level config items
	pprofEnabled       string
	env                string
	ratiosAccessToken  string
	ratiosService      string
	ratiosClientPurge  time.Duration
	ratiosClientExpiry time.Duration
)

// helper to make sure there is no errors
func must(err error) {
	if err != nil {
		log.Printf("failed to initialize: %s\n", err.Error())
	}
}

// Execute - the main entrypoint for all subcommands in bat-go
func Execute() {
	// setup context with logging
	var logger *zerolog.Logger
	ctx, logger = logging.SetupLogger(ctx)

	// execute the root cmd
	if err := rootCmd.Execute(); err != nil {
		logger.Error().Err(err).Msg("./bat-go command encountered an error")
		os.Exit(1)
	}
}

func init() {

	// pprof-enabled - defaults to ""
	rootCmd.PersistentFlags().StringVarP(&pprofEnabled, "pprof-enabled", "", "",
		"pprof enablement")
	must(viper.BindPFlag("pprof-enabled", rootCmd.PersistentFlags().Lookup("pprof-enabled")))
	must(viper.BindEnv("pprof-enabled", "PPROF_ENABLED"))

	// env - defaults to development
	rootCmd.PersistentFlags().StringVarP(&env, "environment", "e", "development",
		"the default environment")
	must(viper.BindPFlag("environment", rootCmd.PersistentFlags().Lookup("environment")))
	must(viper.BindEnv("environment", "ENV"))

	// ratiosAccessToken (required by all)
	rootCmd.PersistentFlags().StringVarP(&ratiosAccessToken, "ratios-token", "t", "",
		"the ratios service token for this service")
	must(viper.BindPFlag("ratios-token", rootCmd.PersistentFlags().Lookup("ratios-token")))
	must(viper.BindEnv("ratios-token", "RATIOS_TOKEN"))

	// ratiosService (required by all)
	rootCmd.PersistentFlags().StringVarP(&ratiosService, "ratios-service", "r", "",
		"the ratios service address")
	must(viper.BindPFlag("ratios-service", rootCmd.PersistentFlags().Lookup("ratios-service")))
	must(viper.BindEnv("ratios-service", "RATIOS_SERVICE"))

	// ratiosClientExpiry
	rootCmd.PersistentFlags().DurationVarP(&ratiosClientExpiry, "ratios-client-cache-expiry", "", 5*time.Second,
		"the ratios client cache default eviction duration")
	must(viper.BindPFlag("ratios-client-cache-expiry", rootCmd.PersistentFlags().Lookup("ratios-client-cache-expiry")))
	must(viper.BindEnv("ratios-client-cache-expiry", "RATIOS_CACHE_EXPIRY"))

	// ratiosClientPurge
	rootCmd.PersistentFlags().DurationVarP(&ratiosClientPurge, "ratios-client-cache-purge", "", 1*time.Minute,
		"the ratios client cache default purge duration")
	must(viper.BindPFlag("ratios-client-cache-purge", rootCmd.PersistentFlags().Lookup("ratios-client-cache-purge")))
	must(viper.BindEnv("ratios-client-cache-purge", "RATIOS_CACHE_PURGE"))
}

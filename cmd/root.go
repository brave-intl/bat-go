package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/brave-intl/bat-go/libs/clients"
	cmdutils "github.com/brave-intl/bat-go/libs/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/logging"
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
)

// Execute - the main entrypoint for all subcommands in bat-go
func Execute(version, commit, buildTime string) {
	// setup context with logging, but first we need to setup the environment
	var logger *zerolog.Logger
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, viper.Get("environment"))
	ctx = context.WithValue(ctx, appctx.DebugLoggingCTXKey, viper.GetBool("debug"))
	ctx, logger = logging.SetupLogger(ctx)
	// setup ratios service values
	ctx = context.WithValue(ctx, appctx.RatiosServerCTXKey, viper.Get("ratios-service"))
	ctx = context.WithValue(ctx, appctx.RatiosAccessTokenCTXKey, viper.Get("ratios-token"))

	ctx = context.WithValue(ctx, appctx.VersionCTXKey, version)
	ctx = context.WithValue(ctx, appctx.CommitCTXKey, commit)
	ctx = context.WithValue(ctx, appctx.BuildTimeCTXKey, buildTime)

	// execute the root cmd
	if err := RootCmd.ExecuteContext(ctx); err != nil {
		logger.Error().Err(err).Msg("./bat-go command encountered an error")
		os.Exit(1)
	}
}

func init() {
	// pprof-enabled - defaults to ""
	RootCmd.PersistentFlags().String("pprof-enabled", "",
		"pprof enablement")
	cmdutils.Must(viper.BindPFlag("pprof-enabled", RootCmd.PersistentFlags().Lookup("pprof-enabled")))
	cmdutils.Must(viper.BindEnv("pprof-enabled", "PPROF_ENABLED"))

	// env - defaults to local
	RootCmd.PersistentFlags().String("environment", "local",
		"the default environment")
	cmdutils.Must(viper.BindPFlag("environment", RootCmd.PersistentFlags().Lookup("environment")))
	cmdutils.Must(viper.BindEnv("environment", "ENV"))

	// debug logging - defaults to off
	RootCmd.PersistentFlags().Bool("debug", false, "turn on debug logging")
	cmdutils.Must(viper.BindPFlag("debug", RootCmd.PersistentFlags().Lookup("debug")))
	cmdutils.Must(viper.BindEnv("debug", "DEBUG"))

	// ratiosAccessToken (required by all)
	RootCmd.PersistentFlags().String("ratios-token", "",
		"the ratios service token for this service")
	cmdutils.Must(viper.BindPFlag("ratios-token", RootCmd.PersistentFlags().Lookup("ratios-token")))
	cmdutils.Must(viper.BindEnv("ratios-token", "RATIOS_TOKEN"))

	// ratiosService (required by all)
	RootCmd.PersistentFlags().String("ratios-service", "",
		"the ratios service address")
	cmdutils.Must(viper.BindPFlag("ratios-service", RootCmd.PersistentFlags().Lookup("ratios-service")))
	cmdutils.Must(viper.BindEnv("ratios-service", "RATIOS_SERVICE"))

	// ratiosClientExpiry
	RootCmd.PersistentFlags().Duration("ratios-client-cache-expiry", 5*time.Second,
		"the ratios client cache default eviction duration")
	cmdutils.Must(viper.BindPFlag("ratios-client-cache-expiry", RootCmd.PersistentFlags().Lookup("ratios-client-cache-expiry")))
	cmdutils.Must(viper.BindEnv("ratios-client-cache-expiry", "RATIOS_CACHE_EXPIRY"))

	// ratiosClientPurge
	RootCmd.PersistentFlags().Duration("ratios-client-cache-purge", 1*time.Minute,
		"the ratios client cache default purge duration")
	cmdutils.Must(viper.BindPFlag("ratios-client-cache-purge", RootCmd.PersistentFlags().Lookup("ratios-client-cache-purge")))
	cmdutils.Must(viper.BindEnv("ratios-client-cache-purge", "RATIOS_CACHE_PURGE"))

	RootCmd.AddCommand(VersionCmd)
}

// VersionCmd is the command to get the code's version information
var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "get the version of this binary",
	Run:   versionRun,
}

func versionRun(command *cobra.Command, args []string) {
	version := command.Context().Value(appctx.VersionCTXKey).(string)
	commit := command.Context().Value(appctx.CommitCTXKey).(string)
	buildTime := command.Context().Value(appctx.BuildTimeCTXKey).(string)
	fmt.Printf("version: %s\ncommit: %s\nbuild time: %s\n",
		version, commit, buildTime,
	)
}

// Perform performs a run
func Perform(action string, fn func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		err := fn(cmd, args)
		if err != nil {
			logger, lerr := appctx.GetLogger(cmd.Context())
			if lerr != nil {
				_, logger = logging.SetupLogger(cmd.Context())
			}

			log := logger.Err(err).Str("action", action)
			httpError, ok := err.(*errorutils.ErrorBundle)
			if ok {
				state, ok := httpError.Data().(clients.HTTPState)
				if ok {
					log = log.Int("status", state.Status).
						Str("path", state.Path).
						Interface("data", state.Body)
				}
			}
			log.Msg("failed")
		}
		<-time.After(10 * time.Millisecond)
		if err != nil {
			os.Exit(1)
		}
	}
}

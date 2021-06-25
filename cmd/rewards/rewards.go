package rewards

import (

	// pprof imports
	"context"
	_ "net/http/pprof"
	"time"

	"github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	// add grpc and rest commands
	rewardsCmd.AddCommand(grpcCmd)
	rewardsCmd.AddCommand(restCmd)

	// add this command as a serve subcommand
	cmd.ServeCmd.AddCommand(rewardsCmd)

	// setup the flags

	// ratiosAccessToken (required by all)
	rewardsCmd.PersistentFlags().String("ratios-token", "",
		"the ratios service token for this service")
	cmd.Must(viper.BindPFlag("ratios-token", rewardsCmd.PersistentFlags().Lookup("ratios-token")))
	cmd.Must(viper.BindEnv("ratios-token", "RATIOS_TOKEN"))

	// ratiosService (required by all)
	rewardsCmd.PersistentFlags().String("ratios-service", "",
		"the ratios service address")
	cmd.Must(viper.BindPFlag("ratios-service", rewardsCmd.PersistentFlags().Lookup("ratios-service")))
	cmd.Must(viper.BindEnv("ratios-service", "RATIOS_SERVICE"))

	// ratiosClientExpiry
	rewardsCmd.PersistentFlags().Duration("ratios-client-cache-expiry", 5*time.Second,
		"the ratios client cache default eviction duration")
	cmd.Must(viper.BindPFlag("ratios-client-cache-expiry", rewardsCmd.PersistentFlags().Lookup("ratios-client-cache-expiry")))
	cmd.Must(viper.BindEnv("ratios-client-cache-expiry", "RATIOS_CACHE_EXPIRY"))

	// ratiosClientPurge
	rewardsCmd.PersistentFlags().Duration("ratios-client-cache-purge", 1*time.Minute,
		"the ratios client cache default purge duration")
	cmd.Must(viper.BindPFlag("ratios-client-cache-purge", rewardsCmd.PersistentFlags().Lookup("ratios-client-cache-purge")))
	cmd.Must(viper.BindEnv("ratios-client-cache-purge", "RATIOS_CACHE_PURGE"))

	// defaultCurrency - defaults to USD
	rewardsCmd.PersistentFlags().String("default-currency", "USD",
		"the default base currency for the rewards system")
	cmd.Must(viper.BindPFlag("default-currency", rewardsCmd.PersistentFlags().Lookup("default-currency")))
	cmd.Must(viper.BindEnv("default-currency", "DEFAULT_CURRENCY"))

	// defaultTipChoices - defaults to 1,10,100
	rewardsCmd.PersistentFlags().String("default-tip-choices", `1,10,100`,
		"the default tip choices for the rewards system")
	cmd.Must(viper.BindPFlag("default-tip-choices", rewardsCmd.PersistentFlags().Lookup("default-tip-choices")))
	cmd.Must(viper.BindEnv("default-tip-choices", "DEFAULT_TIP_CHOICES"))

	// defaultMonthlyChoices - defaults to 1,10,100
	rewardsCmd.PersistentFlags().String("default-monthly-choices", `1,10,100`,
		"the default monthly choices for the rewards system")
	cmd.Must(viper.BindPFlag("default-monthly-choices", rewardsCmd.PersistentFlags().Lookup("default-monthly-choices")))
	cmd.Must(viper.BindEnv("default-monthly-choices", "DEFAULT_MONTHLY_CHOICES"))

	// defaultACChoices - defaults to empty (which causes the choices to be dynamic)
	rewardsCmd.PersistentFlags().String("default-ac-choices", "",
		"the default ac choices for the rewards system")
	cmd.Must(viper.BindPFlag("default-ac-choices", rewardsCmd.PersistentFlags().Lookup("default-ac-choices")))
	cmd.Must(viper.BindEnv("default-ac-choices", "DEFAULT_AC_CHOICES"))

	// defaultACChoice - defaults to empty (which causes the choices to be dynamic)
	rewardsCmd.PersistentFlags().String("default-ac-choice", "",
		"the default ac choice for the rewards system")
	cmd.Must(viper.BindPFlag("default-ac-choice", rewardsCmd.PersistentFlags().Lookup("default-ac-choice")))
	cmd.Must(viper.BindEnv("default-ac-choice", "DEFAULT_AC_CHOICE"))
}

var (
	rewardsCmd = &cobra.Command{
		Use:   "rewards",
		Short: "provides rewards micro-service entrypoint",
	}

	restCmd = &cobra.Command{
		Use:   "rest",
		Short: "provides REST api services",
		Run:   RestRun,
	}

	grpcCmd = &cobra.Command{
		Use:   "grpc",
		Short: "provides gRPC api services",
		Run:   GRPCRun,
	}
)

func CommonRun(command *cobra.Command, args []string) (context.Context, *zerolog.Logger) {
	ctx := command.Context()
	logger, err := appctx.GetLogger(ctx)
	cmd.Must(err)

	// setup ratios service values
	ctx = context.WithValue(ctx, appctx.RatiosServerCTXKey, viper.Get("ratios-service"))
	ctx = context.WithValue(ctx, appctx.RatiosAccessTokenCTXKey, viper.Get("ratios-token"))

	return ctx, logger
}

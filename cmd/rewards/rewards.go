package rewards

import (

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/cmd"
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

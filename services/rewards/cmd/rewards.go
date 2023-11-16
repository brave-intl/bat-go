package cmd

import (

	// pprof imports
	_ "net/http/pprof"

	cmdutils "github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/services/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	rewardsCmd.AddCommand(restCmd)

	// add this command as a serve subcommand
	cmd.ServeCmd.AddCommand(rewardsCmd)

	// setup the flags
	// vbat-deadline - defaults to ""
	rewardsCmd.PersistentFlags().String("vbat-deadline", "",
		"vbat deadline parameter")
	cmdutils.Must(viper.BindPFlag("vbat-deadline", rewardsCmd.PersistentFlags().Lookup("vbat-deadline")))
	cmdutils.Must(viper.BindEnv("vbat-deadline", "VBAT_DEADLINE"))

	// transition- defaults to false
	rewardsCmd.PersistentFlags().Bool("transition", false,
		"transition parameter")
	cmdutils.Must(viper.BindPFlag("transition", rewardsCmd.PersistentFlags().Lookup("transition")))
	cmdutils.Must(viper.BindEnv("transition", "TRANSITION"))

	// merge_param_bucket - defaults to ""
	rewardsCmd.PersistentFlags().String("merge-param-bucket", "",
		"the bucket for which parameters are merged into this service")
	cmdutils.Must(viper.BindPFlag("merge-param-bucket", rewardsCmd.PersistentFlags().Lookup("merge-param-bucket")))
	cmdutils.Must(viper.BindEnv("merge-param-bucket", "MERGE_PARAM_BUCKET"))

	// defaultCurrency - defaults to USD
	rewardsCmd.PersistentFlags().String("default-currency", "USD",
		"the default base currency for the rewards system")
	cmdutils.Must(viper.BindPFlag("default-currency", rewardsCmd.PersistentFlags().Lookup("default-currency")))
	cmdutils.Must(viper.BindEnv("default-currency", "DEFAULT_CURRENCY"))

	// defaultTipChoices - defaults to 1,10,100
	rewardsCmd.PersistentFlags().String("default-tip-choices", `1,10,100`,
		"the default tip choices for the rewards system")
	cmdutils.Must(viper.BindPFlag("default-tip-choices", rewardsCmd.PersistentFlags().Lookup("default-tip-choices")))
	cmdutils.Must(viper.BindEnv("default-tip-choices", "DEFAULT_TIP_CHOICES"))

	// defaultMonthlyChoices - defaults to 1,10,100
	rewardsCmd.PersistentFlags().String("default-monthly-choices", `1,10,100`,
		"the default monthly choices for the rewards system")
	cmdutils.Must(viper.BindPFlag("default-monthly-choices", rewardsCmd.PersistentFlags().Lookup("default-monthly-choices")))
	cmdutils.Must(viper.BindEnv("default-monthly-choices", "DEFAULT_MONTHLY_CHOICES"))

	// defaultACChoices - defaults to empty (which causes the choices to be dynamic)
	rewardsCmd.PersistentFlags().String("default-ac-choices", "",
		"the default ac choices for the rewards system")
	cmdutils.Must(viper.BindPFlag("default-ac-choices", rewardsCmd.PersistentFlags().Lookup("default-ac-choices")))
	cmdutils.Must(viper.BindEnv("default-ac-choices", "DEFAULT_AC_CHOICES"))

	// defaultACChoice - defaults to empty (which causes the choices to be dynamic)
	rewardsCmd.PersistentFlags().String("default-ac-choice", "",
		"the default ac choice for the rewards system")
	cmdutils.Must(viper.BindPFlag("default-ac-choice", rewardsCmd.PersistentFlags().Lookup("default-ac-choice")))
	cmdutils.Must(viper.BindEnv("default-ac-choice", "DEFAULT_AC_CHOICE"))
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
)

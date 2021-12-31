package ratios

import (

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	// add grpc and rest commands
	ratiosCmd.AddCommand(restCmd)

	// add this command as a serve subcommand
	cmd.ServeCmd.AddCommand(ratiosCmd)

	// setup the flags

	ratiosCmd.PersistentFlags().String("coingecko-token", "",
		"the coingecko service token for this service")
	cmd.Must(viper.BindPFlag("coingecko-token", ratiosCmd.PersistentFlags().Lookup("coingecko-token")))
	cmd.Must(viper.BindEnv("coingecko-token", "COINGECKO_TOKEN"))

	ratiosCmd.PersistentFlags().String("coingecko-service", "https://api.coingecko.com/", "the coingecko service address")
	cmd.Must(viper.BindPFlag("coingecko-service", ratiosCmd.PersistentFlags().Lookup("coingecko-service")))
	cmd.Must(viper.BindEnv("coingecko-service", "COINGECKO_SERVICE"))

	ratiosCmd.PersistentFlags().Int("coingecko-coin-limit", 25, "the coingecko coin limit")
	cmd.Must(viper.BindPFlag("coingecko-coin-limit", ratiosCmd.PersistentFlags().Lookup("coingecko-coin-limit")))
	cmd.Must(viper.BindEnv("coingecko-coin-limit", "COINGECKO_COIN_LIMIT"))

	ratiosCmd.PersistentFlags().Int("coingecko-vs-currency-limit", 5, "the coingecko vs currency limit")
	cmd.Must(viper.BindPFlag("coingecko-vs-currency-limit", ratiosCmd.PersistentFlags().Lookup("coingecko-vs-currency-limit")))
	cmd.Must(viper.BindEnv("coingecko-vs-currency-limit", "COINGECKO_VS_CURRENCY_LIMIT"))
}

var (
	ratiosCmd = &cobra.Command{
		Use:   "ratios",
		Short: "provides ratios micro-service entrypoint",
	}

	restCmd = &cobra.Command{
		Use:   "rest",
		Short: "provides REST api services",
		Run:   RestRun,
	}
)

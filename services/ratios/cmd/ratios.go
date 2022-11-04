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
	// add grpc and rest commands
	ratiosCmd.AddCommand(restCmd)

	// add this command as a serve subcommand
	cmd.ServeCmd.AddCommand(ratiosCmd)

	// setup the flags

	ratiosCmd.PersistentFlags().String("coingecko-token", "",
		"the coingecko service token for this service")
	cmdutils.Must(viper.BindPFlag("coingecko-token", ratiosCmd.PersistentFlags().Lookup("coingecko-token")))
	cmdutils.Must(viper.BindEnv("coingecko-token", "COINGECKO_TOKEN"))

	ratiosCmd.PersistentFlags().String("coingecko-service", "https://api.coingecko.com/", "the coingecko service address")
	cmdutils.Must(viper.BindPFlag("coingecko-service", ratiosCmd.PersistentFlags().Lookup("coingecko-service")))
	cmdutils.Must(viper.BindEnv("coingecko-service", "COINGECKO_SERVICE"))

	ratiosCmd.PersistentFlags().Int("coingecko-coin-limit", 25, "the coingecko coin limit")
	cmdutils.Must(viper.BindPFlag("coingecko-coin-limit", ratiosCmd.PersistentFlags().Lookup("coingecko-coin-limit")))
	cmdutils.Must(viper.BindEnv("coingecko-coin-limit", "COINGECKO_COIN_LIMIT"))

	ratiosCmd.PersistentFlags().Int("coingecko-vs-currency-limit", 5, "the coingecko vs currency limit")
	cmdutils.Must(viper.BindPFlag("coingecko-vs-currency-limit", ratiosCmd.PersistentFlags().Lookup("coingecko-vs-currency-limit")))
	cmdutils.Must(viper.BindEnv("coingecko-vs-currency-limit", "COINGECKO_VS_CURRENCY_LIMIT"))

	ratiosCmd.PersistentFlags().String("redis-addr", "redis://localhost:6379", "the redis address")
	cmdutils.Must(viper.BindPFlag("redis-addr", ratiosCmd.PersistentFlags().Lookup("redis-addr")))
	cmdutils.Must(viper.BindEnv("redis-addr", "REDIS_ADDR"))

	ratiosCmd.PersistentFlags().Int("rate-limit-per-min", 50, "rate limit per minute value")
	cmdutils.Must(viper.BindPFlag("rate-limit-per-min", ratiosCmd.PersistentFlags().Lookup("rate-limit-per-min")))
	cmdutils.Must(viper.BindEnv("rate-limit-per-min", "RATE_LIMIT_PER_MIN"))
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

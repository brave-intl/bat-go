package payments

import (

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
)

func init() {
	// add grpc and rest commands
	paymentsCmd.AddCommand(restCmd)

	// add this command as a serve subcommand
	cmd.ServeCmd.AddCommand(paymentsCmd)

	// setup the flags

	//paymentsCmd.PersistentFlags().String("coingecko-token", "",
	//	"the coingecko service token for this service")
	//cmd.Must(viper.BindPFlag("coingecko-token", paymentsCmd.PersistentFlags().Lookup("coingecko-token")))
	//cmd.Must(viper.BindEnv("coingecko-token", "COINGECKO_TOKEN"))

}

var (
	paymentsCmd = &cobra.Command{
		Use:   "payments",
		Short: "provides payments micro-service entrypoint",
	}

	restCmd = &cobra.Command{
		Use:   "rest",
		Short: "provides REST api services",
		Run:   RestRun,
	}
)

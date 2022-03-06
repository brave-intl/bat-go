package subscriptions

import (

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	cmd.RootCmd.AddCommand(subscriptionsCmd)

	// add grpc and rest commands
	subscriptionsCmd.AddCommand(restCmd)

	// add this command as a serve subcommand
	cmd.ServeCmd.AddCommand(subscriptionsCmd)

	// setup the flags
	subscriptionsCmd.PersistentFlags().String("skus-service", "",
		"the skus service address")
	cmd.Must(viper.BindPFlag("skus-service", subscriptionsCmd.PersistentFlags().Lookup("skus-service")))
	cmd.Must(viper.BindEnv("skus-service", "SKUS_SERVICE"))

	subscriptionsCmd.PersistentFlags().String("skus-token", "",
		"the skus service token")
	cmd.Must(viper.BindPFlag("skus-token", subscriptionsCmd.PersistentFlags().Lookup("skus-service")))
	cmd.Must(viper.BindEnv("skus-token", "SKUS_TOKEN"))
}

var (
	subscriptionsCmd = &cobra.Command{
		Use:   "subscriptions",
		Short: "provides subscriptions micro-service entrypoint",
	}

	restCmd = &cobra.Command{
		Use:   "rest",
		Short: "provides REST api services",
		Run:   RestRun,
	}
)

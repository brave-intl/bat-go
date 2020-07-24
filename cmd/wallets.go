package cmd

import (

	// pprof imports
	_ "net/http/pprof"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	walletsCmd = &cobra.Command{
		Use:   "wallet",
		Short: "provides wallets micro-service entrypoint",
	}

	walletsRestCmd = &cobra.Command{
		Use:   "rest",
		Short: "provides REST api services",
		Run:   WalletRestRun,
	}
	db                 string
	walletsFeatureFlag bool
	roDB               string
)

func init() {
	// add grpc and rest commands
	walletsCmd.AddCommand(walletsRestCmd)

	// add this command as a serve subcommand
	serveCmd.AddCommand(walletsCmd)

	// setup the flags
	// datastore - the writable datastore
	walletsCmd.PersistentFlags().StringVarP(&db, "datastore", "", "",
		"the datastore for the wallet system")
	must(viper.BindPFlag("datastore", walletsCmd.PersistentFlags().Lookup("datastore")))
	must(viper.BindEnv("datastore", "DATABASE_URL"))

	// walletsFeatureFlag - enable the wallet endpoints through this feature flag
	walletsCmd.PersistentFlags().BoolVarP(&walletsFeatureFlag, "wallets-feature-flag", "", false,
		"the feature flag enabling the wallets feature")
	must(viper.BindPFlag("wallets-feature-flag", walletsCmd.PersistentFlags().Lookup("wallets-feature-flag")))
	must(viper.BindEnv("wallets-feature-flag", "FEATURE_WALLET"))

	// ro-datastore - the writable datastore
	walletsCmd.PersistentFlags().StringVarP(&roDB, "ro-datastore", "", "",
		"the read only datastore for the wallet system")
	must(viper.BindPFlag("ro-datastore", walletsCmd.PersistentFlags().Lookup("ro-datastore")))
	must(viper.BindEnv("ro-datastore", "RO_DATABASE_URL"))

}

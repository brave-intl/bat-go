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
	db                        string
	walletsFeatureFlag        bool
	walletsInMigrationFlag    bool
	enableLinkingDrainingFlag bool
	roDB                      string
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

	// walletsInMigrationFlag - enable the wallet endpoints through this in migration flag
	walletsCmd.PersistentFlags().BoolVarP(&walletsInMigrationFlag, "wallets-in-migration-flag", "", false,
		"the in-migration flag disabling the wallets link feature")
	must(viper.BindPFlag("wallets-in-migration-flag", walletsCmd.PersistentFlags().Lookup("wallets-in-migration-flag")))
	must(viper.BindEnv("wallets-in-migration-flag", "WALLETS_IN_MIGRATION"))

	// ENABLE_LINKING_DRAINING - enable ability to link wallets and drain wallets
	walletsCmd.PersistentFlags().BoolVarP(&enableLinkingDrainingFlag, "enable-linking-draining-flag", "", false,
		"the in-migration flag disabling the wallets link feature")
	must(viper.BindPFlag("enable-linking-draining-flag", walletsCmd.PersistentFlags().Lookup("enable-linking-draining-flag")))
	must(viper.BindEnv("enable-linking-draining-flag", "ENABLE_LINKING_DRAINING"))

	// ro-datastore - the writable datastore
	walletsCmd.PersistentFlags().StringVarP(&roDB, "ro-datastore", "", "",
		"the read only datastore for the wallet system")
	must(viper.BindPFlag("ro-datastore", walletsCmd.PersistentFlags().Lookup("ro-datastore")))
	must(viper.BindEnv("ro-datastore", "RO_DATABASE_URL"))

}

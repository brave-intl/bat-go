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
	db                string
	roDB              string
	ledgerService     string
	ledgerAccessToken string
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

	// ro-datastore - the writable datastore
	walletsCmd.PersistentFlags().StringVarP(&roDB, "ro-datastore", "", "",
		"the read only datastore for the wallet system")
	must(viper.BindPFlag("ro-datastore", walletsCmd.PersistentFlags().Lookup("ro-datastore")))
	must(viper.BindEnv("ro-datastore", "RO_DATABASE_URL"))

	// ledgerAccessToken (required by all)
	walletsCmd.PersistentFlags().StringVarP(&ledgerAccessToken, "ledger-token", "", "",
		"the ledger service token for this service")
	must(viper.BindPFlag("ledger-token", walletsCmd.PersistentFlags().Lookup("ledger-token")))
	must(viper.BindEnv("ledger-token", "LEDGER_TOKEN"))

	// ledgerService (required by all)
	walletsCmd.PersistentFlags().StringVarP(&ledgerService, "ledger-service", "", "",
		"the ledger service address")
	must(viper.BindPFlag("ledger-service", walletsCmd.PersistentFlags().Lookup("ledger-service")))
	must(viper.BindEnv("ledger-service", "LEDGER_SERVICE"))
	must(viper.BindEnv("ledger-service", "LEDGER_SERVER"))
}

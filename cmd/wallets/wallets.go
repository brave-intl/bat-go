package wallets

import (

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// WalletsCmd root wallets command
	WalletsCmd = &cobra.Command{
		Use:   "wallet",
		Short: "provides wallets micro-service entrypoint",
	}

	walletsRestCmd = &cobra.Command{
		Use:   "rest",
		Short: "provides REST api services",
		Run:   WalletRestRun,
	}
	db                  string
	walletsFeatureFlag  bool
	enableLinkDrainFlag bool
	roDB                string
	bfJWTKey            string
)

func init() {
	// add grpc and rest commands
	WalletsCmd.AddCommand(walletsRestCmd)

	// add this command as a serve subcommand
	cmd.RootCmd.AddCommand(WalletsCmd)

	// setup the flags

	// bitflier-jwt-key - the jwt validation key from bf
	WalletsCmd.PersistentFlags().StringVarP(&bfJWTKey, "bitflier-jwt-key", "", "",
		"the bitflier jwt key for validation of linking info")
	cmd.Must(viper.BindPFlag("bitflier-jwt-key", WalletsCmd.PersistentFlags().Lookup("bitflier-jwt-key")))
	cmd.Must(viper.BindEnv("bitflier-jwt-key", "BITFLIER_JWT_KEY"))

	// datastore - the writable datastore
	WalletsCmd.PersistentFlags().StringVarP(&db, "datastore", "", "",
		"the datastore for the wallet system")
	cmd.Must(viper.BindPFlag("datastore", WalletsCmd.PersistentFlags().Lookup("datastore")))
	cmd.Must(viper.BindEnv("datastore", "DATABASE_URL"))

	// walletsFeatureFlag - enable the wallet endpoints through this feature flag
	WalletsCmd.PersistentFlags().BoolVarP(&walletsFeatureFlag, "wallets-feature-flag", "", false,
		"the feature flag enabling the wallets feature")
	cmd.Must(viper.BindPFlag("wallets-feature-flag", WalletsCmd.PersistentFlags().Lookup("wallets-feature-flag")))
	cmd.Must(viper.BindEnv("wallets-feature-flag", "FEATURE_WALLET"))

	// ENABLE_LINKING_DRAINING - enable ability to link wallets and drain wallets
	WalletsCmd.PersistentFlags().BoolVarP(&enableLinkDrainFlag, "enable-link-drain-flag", "", false,
		"the in-migration flag disabling the wallets link feature")
	cmd.Must(viper.BindPFlag("enable-link-drain-flag", WalletsCmd.PersistentFlags().Lookup("enable-link-drain-flag")))
	cmd.Must(viper.BindEnv("enable-link-drain-flag", "ENABLE_LINKING_DRAINING"))

	// ro-datastore - the writable datastore
	WalletsCmd.PersistentFlags().StringVarP(&roDB, "ro-datastore", "", "",
		"the read only datastore for the wallet system")
	cmd.Must(viper.BindPFlag("ro-datastore", WalletsCmd.PersistentFlags().Lookup("ro-datastore")))
	cmd.Must(viper.BindEnv("ro-datastore", "RO_DATABASE_URL"))
}

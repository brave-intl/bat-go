package wallets

import (

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
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
)

func init() {
	// add grpc and rest commands
	WalletsCmd.AddCommand(walletsRestCmd)

	// add this command as a serve subcommand
	cmd.RootCmd.AddCommand(WalletsCmd)

	// setup the flags
	walletsCmdBuilder := cmd.NewFlagBuilder(WalletsCmd)

	walletsCmdBuilder.Flag().String("bitflyer-jwt-key", "",
		"the bitflyer jwt key for validation of linking info").
		Env("BITFLYER_JWT_KEY").
		Bind("bitflyer-jwt-key").
		Require()

	walletsCmdBuilder.Flag().String("ro-datastore", "",
		"the read only datastore for the wallet system").
		Env("RO_DATABASE_URL").
		Bind("ro-datastore").
		Require()

	walletsCmdBuilder.Flag().String("datastore", "",
		"the datastore for the wallet system").
		Env("DATABASE_URL").
		Bind("datastore").
		Require()

	walletsCmdBuilder.Flag().Bool("wallets-feature-flag", false,
		"the feature flag enabling the wallets feature").
		Env("FEATURE_WALLET").
		Bind("wallets-feature-flag").
		Require()

	walletsCmdBuilder.Flag().Bool("enable-link-drain-flag", false,
		"the in-migration flag disabling the wallets link feature").
		Env("ENABLE_LINKING_DRAINING").
		Bind("enable-link-drain-flag").
		Require()

}

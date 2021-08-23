package skuscli

import (

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
)

var (
	// SkusCmd root skus command
	SkusCmd = &cobra.Command{
		Use:   "skus",
		Short: "provides skus micro-service entrypoint",
	}

	skusRestCmd = &cobra.Command{
		Use:   "rest",
		Short: "provides REST api services",
		Run:   SkusRestRun,
	}
)

func init() {
	// add grpc and rest commands
	SkusCmd.AddCommand(skusRestCmd)

	// add this command as a serve subcommand
	cmd.RootCmd.AddCommand(SkusCmd)

	// setup the flags
	skusCmdBuilder := cmd.NewFlagBuilder(SkusCmd)

	skusCmdBuilder.Flag().String("bitflyer-jwt-key", "",
		"the bitflyer jwt key for validation of linking info").
		Env("BITFLYER_JWT_KEY").
		Bind("bitflyer-jwt-key").
		Require()

	skusCmdBuilder.Flag().String("ro-datastore", "",
		"the read only datastore for the skus system").
		Env("RO_DATABASE_URL").
		Bind("ro-datastore").
		Require()

	skusCmdBuilder.Flag().String("datastore", "",
		"the datastore for the skus system").
		Env("DATABASE_URL").
		Bind("datastore").
		Require()

	skusCmdBuilder.Flag().StringSlice("skus-whitelist", []string{""},
		"the whitelist of skus").
		Bind("skus-whitelist").
		Env("SKUS_WHITELIST")

	skusCmdBuilder.Flag().String("wallet-on-platform-prior-to", "",
		"wallet on platform prior to for transfer").
		Bind("wallet-on-platform-prior-to").
		Env("WALLET_ON_PLATFORM_PRIOR_TO")

	skusCmdBuilder.Flag().Bool("reputation-on-drain", false,
		"check wallet reputation on drain").
		Bind("reputation-on-drain").
		Env("REPUTATION_ON_DRAIN")

	// stripe configurations
	skusCmdBuilder.Flag().Bool("stripe-enabled", false,
		"is stripe enabled for skus").
		Bind("stripe-enabled").
		Env("STRIPE_ENABLED")

	skusCmdBuilder.Flag().String("stripe-webhook-secret", "",
		"the stripe webhook secret").
		Bind("stripe-webhook-secret").
		Env("STRIPE_WEBHOOK_SECRET")

	skusCmdBuilder.Flag().String("stripe-secret", "",
		"the stripe secret").
		Bind("stripe-secret").
		Env("STRIPE_SECRET")
}

package payments

import (

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// PaymentsCmd root payments command
	PaymentsCmd = &cobra.Command{
		Use:   "payment",
		Short: "provides payment micro-service entrypoint",
	}

	paymentsRestCmd = &cobra.Command{
		Use:   "rest",
		Short: "provides REST api services",
		Run:   PaymentRestRun,
	}
	db                  string
	paymentsFeatureFlag bool
	enableLinkDrainFlag bool
	roDB                string
)

func init() {
	// add grpc and rest commands
	PaymentsCmd.AddCommand(paymentsRestCmd)

	// add this command as a serve subcommand
	cmd.RootCmd.AddCommand(PaymentsCmd)

	// setup the flags
	// datastore - the writable datastore
	PaymentsCmd.PersistentFlags().StringVarP(&db, "datastore", "", "",
		"the datastore for the payment system")
	cmd.Must(viper.BindPFlag("datastore", PaymentsCmd.PersistentFlags().Lookup("datastore")))
	cmd.Must(viper.BindEnv("datastore", "DATABASE_URL"))

	// paymentsFeatureFlag - enable the payment endpoints through this feature flag
	PaymentsCmd.PersistentFlags().BoolVarP(&paymentsFeatureFlag, "payments-feature-flag", "", false,
		"the feature flag enabling the payments feature")
	cmd.Must(viper.BindPFlag("payments-feature-flag", PaymentsCmd.PersistentFlags().Lookup("payments-feature-flag")))
	cmd.Must(viper.BindEnv("payments-feature-flag", "FEATURE_PAYMENT"))

	// ENABLE_LINKING_DRAINING - enable ability to link payments and drain payments
	PaymentsCmd.PersistentFlags().BoolVarP(&enableLinkDrainFlag, "enable-link-drain-flag", "", false,
		"the in-migration flag disabling the payments link feature")
	cmd.Must(viper.BindPFlag("enable-link-drain-flag", PaymentsCmd.PersistentFlags().Lookup("enable-link-drain-flag")))
	cmd.Must(viper.BindEnv("enable-link-drain-flag", "ENABLE_LINKING_DRAINING"))

	// ro-datastore - the writable datastore test
	PaymentsCmd.PersistentFlags().StringVarP(&roDB, "ro-datastore", "", "",
		"the read only datastore for the payment system")
	cmd.Must(viper.BindPFlag("ro-datastore", PaymentsCmd.PersistentFlags().Lookup("ro-datastore")))
	cmd.Must(viper.BindEnv("ro-datastore", "RO_DATABASE_URL"))
}

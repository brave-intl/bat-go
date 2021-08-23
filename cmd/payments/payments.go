package payments

import (

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	// add grpc and rest commands
	paymentsCmd.AddCommand(grpcCmd)
	paymentsCmd.AddCommand(restCmd)

	// add this command as a serve subcommand
	cmd.ServeCmd.AddCommand(paymentsCmd)

	// setup the flags

	// batch-sign-keypair - defaults to USD
	paymentsCmd.PersistentFlags().String("batch-sign-keypair", "",
		"the key pair used to sign batches")
	cmd.Must(viper.BindPFlag("batch-sign-keypair", paymentsCmd.PersistentFlags().Lookup("batch-sign-keypair")))
	cmd.Must(viper.BindEnv("batch-sign-keypair", "BATCH_SIGN_KEYPAIR"))

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

	grpcCmd = &cobra.Command{
		Use:   "grpc",
		Short: "provides gRPC api services",
		Run:   GRPCRun,
	}
)

package cmd

import (

	// pprof imports
	_ "net/http/pprof"

	srvcmd "github.com/brave-intl/bat-go/services/cmd"
	"github.com/spf13/cobra"
)

func init() {
	// add grpc and rest commands
	paymentsCmd.AddCommand(restCmd)

	// add worker command
	paymentsCmd.AddCommand(workersCmd)

	// add this command as a serve subcommand
	srvcmd.ServeCmd.AddCommand(paymentsCmd)

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

	workersCmd = &cobra.Command{
		Use:   "worker",
		Short: "provides redis stream worker",
		Run:   WorkerRun,
	}
)

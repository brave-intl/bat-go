package subscriptions

import (

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(subscriptionsCmd)

	// add grpc and rest commands
	subscriptionsCmd.AddCommand(restCmd)

	// add this command as a serve subcommand
	cmd.ServeCmd.AddCommand(subscriptionsCmd)

	// setup the flags
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

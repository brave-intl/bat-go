package swaps

import (
	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
)

func init() {
	// add this command as a serve subcommand
	cmd.ServeCmd.AddCommand(swapsCmd) // TODO should this be Serve? It's not a server
}

var (
	swapsCmd = &cobra.Command{
		Use:   "swaps",
		Short: "provides swaps micro-service entrypoint",
		Run:   Run,
	}
)

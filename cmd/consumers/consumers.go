package consumers

import (
	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
)

var (
	// ConsumersCmd a subcommand for consumers
	ConsumersCmd = &cobra.Command{
		Use:   "consumers",
		Short: "subcommand to start a given job",
	}
)

func init() {
	cmd.RootCmd.AddCommand(ConsumersCmd)
}

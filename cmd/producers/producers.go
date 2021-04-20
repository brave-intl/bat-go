package consumers

import (
	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
)

var (
	// ProducersCmd the subcommand to produce messages
	ProducersCmd = &cobra.Command{
		Use:   "producers",
		Short: "subcommand to produce messages to a topic",
	}
)

func init() {
	cmd.RootCmd.AddCommand(ProducersCmd)
}

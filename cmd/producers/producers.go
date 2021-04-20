package consumers

import (
	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
)

var (
	ProducersCmd = &cobra.Command{
		Use:   "producers",
		Short: "subcommand to start a given job",
	}
)

func init() {
	cmd.RootCmd.AddCommand(ProducersCmd)
}

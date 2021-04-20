package jobs

import (
	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
)

var (
	JobsCmd = &cobra.Command{
		Use:   "jobs",
		Short: "subcommand to start a given job",
	}
)

func init() {
	cmd.RootCmd.AddCommand(JobsCmd)
}

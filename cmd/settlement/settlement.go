package settlement

import (
	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
)

func init() {
	cmd.RootCmd.AddCommand(settlementCmd)
}

var settlementCmd = &cobra.Command{
	Use:   "settlement",
	Short: "provides settlement utilities",
}

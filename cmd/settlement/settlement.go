package settlement

import (
	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
)

func init() {
	cmd.RootCmd.AddCommand(SettlementCmd)
}

// SettlementCmd is the settlement command
var SettlementCmd = &cobra.Command{
	Use:   "settlement",
	Short: "provides settlement utilities",
}

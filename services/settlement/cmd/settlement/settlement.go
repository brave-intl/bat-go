package settlement

import (
	"github.com/brave-intl/bat-go/services/cmd"
	"github.com/spf13/cobra"
)

func init() {
	cmd.ServeCmd.AddCommand(Cmd)
}

// Cmd is the settlement command.
var Cmd = &cobra.Command{
	Use:   "settlement",
	Short: "provides settlement utilities",
}

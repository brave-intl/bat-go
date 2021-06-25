package nitro

import (
	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
)

func init() {
	NitroServeCmd.AddCommand(OpenProxyServerCmd)
	NitroServeCmd.AddCommand(ReverseProxyServerCmd)

	cmd.ServeCmd.AddCommand(NitroServeCmd)
}

// NitroServeCmd the nitro serve command
var NitroServeCmd = &cobra.Command{
	Use:   "nitro",
	Short: "subcommand to serve a nitro micro-service",
}

package vault

import (
	cmdutils "github.com/brave-intl/bat-go/cmd"
	rootcmd "github.com/brave-intl/bat-go/cmd"
	settlement "github.com/brave-intl/bat-go/tools/settlement"
	"github.com/spf13/cobra"
)

var (
	// Config is a configuration file to map known wallet keys to unknown wallet keys
	Config *settlement.Config

	// VaultCmd adds a command to cobra for vault interfacing
	VaultCmd = &cobra.Command{
		Use:   "vault",
		Short: "provides a succinct interface with vault",
	}
)

func init() {
	rootcmd.RootCmd.AddCommand(VaultCmd)
}

// ReadConfig sets up the config flag
func ReadConfig(command *cobra.Command) *settlement.Config {
	configPath, err := command.Flags().GetString("config")
	cmdutils.Must(err)
	config, err := settlement.ReadYamlConfig(configPath)
	cmdutils.Must(err)
	Config = config
	return config
}

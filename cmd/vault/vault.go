package vault

import (
	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/settlement"
	"github.com/spf13/cobra"
)

var (
	// Config is a configuration file to map provider + transaction type to the appropriate vault wallet
	Config *settlement.Config

	// VaultCmd adds a command to cobra for vault interfacing
	VaultCmd = &cobra.Command{
		Use:   "vault",
		Short: "provides a succinct interface with vault",
	}
)

func init() {
	cmd.RootCmd.AddCommand(VaultCmd)
}

// ReadConfig sets up the config flag
func ReadConfig(command *cobra.Command) *settlement.Config {
	configPath, err := command.Flags().GetString("config")
	cmd.Must(err)
	config, err := settlement.ReadYamlConfig(configPath)
	cmd.Must(err)
	Config = config
	return config
}

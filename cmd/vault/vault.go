package vault

import (
	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/settlement"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	cmd.RootCmd.AddCommand(VaultCmd)

	// config - defaults to config.yaml
	VaultCmd.PersistentFlags().String("config", "config.yaml",
		"the default path to a configuration file")
	cmd.Must(viper.BindPFlag("config", VaultCmd.PersistentFlags().Lookup("config")))
	cmd.Must(viper.BindEnv("config", "CONFIG"))
}

// ReadConfig sets up the config flag
func ReadConfig(command *cobra.Command) *settlement.Config {
	config, err := settlement.ReadYamlConfig(viper.GetString("config"))
	cmd.Must(err)
	Config = config
	return config
}

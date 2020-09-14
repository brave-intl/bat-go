package vault

import (
	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/settlement"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// ConfigPath provides a path to read configuration out of
	ConfigPath string

	// Config is a configuration file to map known wallet keys to unknown wallet keys
	Config *settlement.Config

	// VaultCmd adds a command to cobra for vault interfacing
	VaultCmd = &cobra.Command{
		Use:   "vault",
		Short: "provides a succinct interface with vault",
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	cmd.RootCmd.AddCommand(VaultCmd)

	// config - defaults to config.yaml
	VaultCmd.PersistentFlags().StringVarP(&ConfigPath, "config", "c", "config.yaml",
		"the default path to a configuration file")
	cmd.Must(viper.BindPFlag("config", VaultCmd.PersistentFlags().Lookup("config")))
	cmd.Must(viper.BindEnv("config", "CONFIG"))
	// cmd.Must(VaultCmd.MarkFlagRequired("config"))
}

func initConfig() {
	var err error
	Config, err = settlement.ReadYamlConfig(ConfigPath)
	cmd.Must(err)
}

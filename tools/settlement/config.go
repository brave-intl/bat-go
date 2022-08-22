package settlement

import (
	"os"
	"os/user"
	"path"

	"gopkg.in/yaml.v2"
)

// Config a space for complex inputs
type Config struct {
	Wallets map[string]string `yaml:"wallets"`
}

// GetWalletKey accesses the wallet config
func (config *Config) GetWalletKey(key string) string {
	value := config.Wallets[key]
	if value == "" {
		return key
	}
	return value
}

// ReadYamlConfig reads a yaml config
func ReadYamlConfig(configPath string) (*Config, error) {
	if configPath == "" {
		usr, err := user.Current()
		if err != nil {
			return nil, err
		}
		configPath = path.Join(usr.HomeDir, ".settlement.yaml")
	}
	// Open config file
	var config Config
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	// Init new YAML decode
	d := yaml.NewDecoder(file)

	// Start YAML decoding from file
	if err := d.Decode(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

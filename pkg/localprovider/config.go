package localprovider

import "fmt"

// LocalProviderConfig provides a configmanager for local provider
type LocalProviderConfig struct {
	ConfigManager ConfigManager `yaml:"configManager"`
}

// ConfigManager provides a LocalConfigName
type ConfigManager struct {
	LocalConfigName string `yaml:"localConfigName"`
}

// IsValid returns no error when the local provider config is valid
func (lp *LocalProviderConfig) IsValid() error {
	cfg := lp.ConfigManager.LocalConfigName
	if cfg != UPGRADECONFIG_CR_NAME {
		return fmt.Errorf("please use %s as the upgrade config name", UPGRADECONFIG_CR_NAME)
	}

	return nil
}

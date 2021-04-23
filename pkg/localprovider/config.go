package localprovider

import "fmt"

type LocalProviderConfig struct {
	ConfigManager ConfigManager `yaml:"configManager"`
}

type ConfigManager struct {
	LocalConfigName string `yaml:"localConfigName"`
}

func (lp *LocalProviderConfig) IsValid() error {
	cfg := lp.ConfigManager.LocalConfigName
	if cfg != UPGRADECONFIG_CR_NAME {
		return fmt.Errorf("please use %s as the upgrade config name", UPGRADECONFIG_CR_NAME)
	}

	return nil
}

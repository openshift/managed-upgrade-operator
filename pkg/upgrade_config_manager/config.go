package upgrade_config_manager

import "fmt"

const (
	OCM ConfigManagerSource = "OCM"
)

type upgradeConfigManagerConfig struct {
	ConfigManager configManager `yaml:"configManager"`
}

type configManager struct {
	Source string `yaml:"source"`
}

var ErrNoConfigManagerDefined = fmt.Errorf("no configManager defined in configuration")

func (cfg *upgradeConfigManagerConfig) IsValid() error {
	switch cfg.ConfigManager.Source {
	case string(OCM):
		return nil
	default:
		return ErrNoConfigManagerDefined
	}
}

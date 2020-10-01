package policyprovider

import (
	"fmt"
)

const (
	OCM ConfigManagerSource = "OCM"
)

type ConfigManagerSource string

type PolicyProviderConfig struct {
	ConfigManager ConfigManager `yaml:"configManager"`
}

type ConfigManager struct {
	Source string `yaml:"source"`
}

var ErrNoPolicyProviderDefined = fmt.Errorf("no configManager policy provider defined in configuration")

func (cfg *PolicyProviderConfig) IsValid() error {
	switch cfg.ConfigManager.Source {
	case string(OCM):
		return nil
	default:
		return ErrNoPolicyProviderDefined
	}
}

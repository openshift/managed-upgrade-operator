package specprovider

import (
	"fmt"
)

const (
	OCM ConfigManagerSource = "OCM"
)

type ConfigManagerSource string

type SpecProviderConfig struct {
	ConfigManager ConfigManager `yaml:"configManager"`
}

type ConfigManager struct {
	Source string `yaml:"source"`
}

var ErrNoSpecProviderDefined = fmt.Errorf("no configManager spec provider defined in configuration")

func (cfg *SpecProviderConfig) IsValid() error {
	switch cfg.ConfigManager.Source {
	case string(OCM):
		return nil
	default:
		return ErrNoSpecProviderDefined
	}
}

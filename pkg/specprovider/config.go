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

var (
	ErrInvalidSpecProvider = fmt.Errorf("invalid configManager spec provider type defined")
	ErrNoSpecProviderConfig     = fmt.Errorf("no configManager spec provider configured")

)

func (cfg *SpecProviderConfig) IsValid() error {
	// the source can be missing. if it's not empty, validate it is a supported value
	if cfg.ConfigManager.Source == "" {
		return ErrNoSpecProviderConfig
	}

	switch cfg.ConfigManager.Source {
	case string(OCM):
		return nil
	default:
		return ErrInvalidSpecProvider
	}
}

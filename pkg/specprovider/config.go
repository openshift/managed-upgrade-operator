package specprovider

import (
	"fmt"
	"strings"
)

const (
	// OCM denotes OCM as the config manager source
	OCM ConfigManagerSource = "OCM"
	// LOCAL denotes a local config manager source
	LOCAL ConfigManagerSource = "LOCAL"
)

// ConfigManagerSource is a type that denotes the source of configuration management
type ConfigManagerSource string

// SpecProviderConfig holds fields that describe a spec providers config
type SpecProviderConfig struct {
	ConfigManager ConfigManager `yaml:"configManager"`
}

// ConfigManager holds fields that describe a ConfigManager
type ConfigManager struct {
	Source string `yaml:"source"`
}

var (
	// ErrInvalidSpecProvider is an error advising an invalid spec provider configuration
	ErrInvalidSpecProvider = fmt.Errorf("invalid configManager spec provider type defined")
	// ErrNoSpecProviderConfig is an error advising no config has been configured
	ErrNoSpecProviderConfig = fmt.Errorf("no configManager spec provider configured")
)

// IsValid returns a nil error when the SpecProviderConfig is valid
func (cfg *SpecProviderConfig) IsValid() error {
	// the source can be missing. if it's not empty, validate it is a supported value
	if cfg.ConfigManager.Source == "" {
		return ErrNoSpecProviderConfig
	}

	switch strings.ToUpper(cfg.ConfigManager.Source) {
	case string(OCM):
		return nil
	case string(LOCAL):
		return nil
	default:
		return ErrInvalidSpecProvider
	}
}

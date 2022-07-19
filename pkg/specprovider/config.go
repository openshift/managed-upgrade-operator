package specprovider

import (
	"fmt"
	"strings"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
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
	// UpgradeType is used to select which upgrader to use when upgrading
	UpgradeType   string        `yaml:"upgradeType"`
	// ConfigManager configures the source that the operator uses to read UpgradeConfig specs
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
		break
	case string(LOCAL):
		break
	default:
		return ErrInvalidSpecProvider
	}

	switch upgradev1alpha1.UpgradeType(cfg.UpgradeType) {
	case upgradev1alpha1.ARO, upgradev1alpha1.OSD, "":
		// An empty upgrade type is fine
		break
	default:
		return ErrInvalidSpecProvider
	}
	return nil
}

// GetUpgradeType returns the upgrader type to populate in the upgrade config spec
func (cfg *SpecProviderConfig) GetUpgradeType() upgradev1alpha1.UpgradeType {
	// If an upgrade type is not specified, default to ARO
	if len(cfg.UpgradeType) == 0 {
		return upgradev1alpha1.ARO
	}
	return upgradev1alpha1.UpgradeType(cfg.UpgradeType)
}

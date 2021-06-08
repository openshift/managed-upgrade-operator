package upgradeconfigmanager

import (
	"fmt"
	"time"
)

// UpgradeConfigManagerConfig holds fields that describe an UpgradeConfigManagerConfig
type UpgradeConfigManagerConfig struct {
	ConfigManager ConfigManager `yaml:"configManager"`
}

// ConfigManager holds fileds that describe a ConfigManager
type ConfigManager struct {
	WatchIntervalMinutes int `yaml:"watchInterval" default:"1"`
}

// ErrNoConfigManagerDefined is an error that describes no configmanager has been configured
var ErrNoConfigManagerDefined = fmt.Errorf("no configManager defined in configuration")

// IsValid returns a nil error when the UpgradeConfigManagerConfig is valid
func (cfg *UpgradeConfigManagerConfig) IsValid() error {
	if cfg.ConfigManager.WatchIntervalMinutes <= 0 {
		return ErrNoConfigManagerDefined
	}
	return nil
}

// GetWatchInterval returns the WatchIntervalMinutes field from an UpgradeConfigManagerConfig
func (cfg *UpgradeConfigManagerConfig) GetWatchInterval() time.Duration {
	return time.Duration(cfg.ConfigManager.WatchIntervalMinutes) * time.Minute
}

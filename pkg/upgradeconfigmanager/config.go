package upgradeconfigmanager

import (
	"fmt"
	"time"
)

type UpgradeConfigManagerConfig struct {
	ConfigManager ConfigManager `yaml:"configManager"`
}

type ConfigManager struct {
	WatchIntervalMinutes int    `yaml:"watchInterval" default:"1"`
}

var ErrNoConfigManagerDefined = fmt.Errorf("no configManager defined in configuration")

func (cfg *UpgradeConfigManagerConfig) IsValid() error {
	if cfg.ConfigManager.WatchIntervalMinutes <= 0 {
		return ErrNoConfigManagerDefined
	}
	return nil
}

func (cfg *UpgradeConfigManagerConfig) GetWatchInterval() time.Duration {
	return time.Duration(cfg.ConfigManager.WatchIntervalMinutes) * time.Minute
}

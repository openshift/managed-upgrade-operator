package ocm_upgrade_config_manager

import (
	"fmt"
	"net/url"
	"time"
)

type ocmUpgradeConfigManagerConfig struct {
	ConfigManagerConfig configManagerConfig `yaml:"configManager"`
}

type configManagerConfig struct {
	OcmBaseURL          string              `yaml:"ocmBaseUrl"`
	WatchInterval int `yaml:"watchInterval" default:"1"`
}

func (cfg *ocmUpgradeConfigManagerConfig) IsValid() error {
	if _, err := url.Parse(cfg.ConfigManagerConfig.OcmBaseURL); err != nil {
		return fmt.Errorf("OCM Base URL is not a parseable URL")
	}
	if cfg.ConfigManagerConfig.WatchInterval <= 0 {
		return fmt.Errorf("Config configManager WatchInterval is invalid")
	}

	return nil
}

func (cfg *ocmUpgradeConfigManagerConfig) GetWatchInterval() time.Duration {
	return time.Duration(cfg.ConfigManagerConfig.WatchInterval) * time.Minute
}

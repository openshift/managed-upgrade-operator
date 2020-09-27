package ocmmanager

import (
	"fmt"
	"net/url"
	"time"
)

type ocmUpgradeConfigManagerConfig struct {
	ConfigManagerConfig configManagerConfig `yaml:"configManager"`
}

type configManagerConfig struct {
	OcmBaseURL           string `yaml:"ocmBaseUrl"`
	WatchIntervalMinutes int    `yaml:"watchInterval" default:"1"`
}

func (cfg *ocmUpgradeConfigManagerConfig) IsValid() error {
	if _, err := url.Parse(cfg.ConfigManagerConfig.OcmBaseURL); err != nil {
		return fmt.Errorf("OCM Base URL is not a parseable URL")
	}
	if cfg.ConfigManagerConfig.WatchIntervalMinutes <= 0 {
		return fmt.Errorf("Config configManager WatchIntervalMinutes is invalid")
	}

	return nil
}

func (cfg *ocmUpgradeConfigManagerConfig) GetOCMBaseURL() (*url.URL, error) {
	url, err := url.Parse(cfg.ConfigManagerConfig.OcmBaseURL)
	return url, err
}

func (cfg *ocmUpgradeConfigManagerConfig) GetWatchInterval() time.Duration {
	return time.Duration(cfg.ConfigManagerConfig.WatchIntervalMinutes) * time.Minute
}

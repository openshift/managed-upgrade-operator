package ocmprovider

import (
	"fmt"
	"net/url"
)

// OcmProviderConfig holds configuration for an OCM provider
type OcmProviderConfig struct {
	ConfigManager ConfigManager `yaml:"configManager"`
}

// ConfigManager manages config for an OCM provider
type ConfigManager struct {
	OcmBaseUrl string `yaml:"ocmBaseUrl"`
}

// IsValid returns a nil error when the OcmProviderConfig is true
func (cfg *OcmProviderConfig) IsValid() error {
	if _, err := url.Parse(cfg.ConfigManager.OcmBaseUrl); err != nil {
		return fmt.Errorf("OCM Base URL is not a parseable URL")
	}
	return nil
}

// GetOCMBaseURL returns the OCM providers base URL from the OCM config
func (cfg *OcmProviderConfig) GetOCMBaseURL() *url.URL {
	url, _ := url.Parse(cfg.ConfigManager.OcmBaseUrl)
	return url
}

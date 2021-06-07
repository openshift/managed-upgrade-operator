package ocm

import (
	"fmt"
	"net/url"
)

// OcmClientConfig holds config for an ocm client
type OcmClientConfig struct {
	ConfigManager ConfigManager `yaml:"configManager"`
}

// ConfigManager holds config for ocm client
type ConfigManager struct {
	OcmBaseUrl string `yaml:"ocmBaseUrl"`
}

// IsValid returns no error if the ocm client config is valid
func (cfg *OcmClientConfig) IsValid() error {
	if _, err := url.Parse(cfg.ConfigManager.OcmBaseUrl); err != nil {
		return fmt.Errorf("OCM Base URL is not a parseable URL")
	}
	return nil
}

// GetOCMBaseURL returns the URL of OCM
func (cfg *OcmClientConfig) GetOCMBaseURL() *url.URL {
	url, _ := url.Parse(cfg.ConfigManager.OcmBaseUrl)
	return url
}

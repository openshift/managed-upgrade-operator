package notifier

import (
	"fmt"
	"net/url"
)

// OcmNotifierConfig holds a ConfigManager field for its OCM configuration
type OcmNotifierConfig struct {
	ConfigManager OcmNotifierConfigManager `yaml:"configManager"`
}

// OcmFeatureConfig holds the OCMFeatureGate field for its features configuration
type OcmFeatureConfig struct {
	OCMFeatureGate OcmFeatureGates `yaml:"featureGate"`
}

// OcmFeatureGates holds the list of featuregates that are enabled for OCM
type OcmFeatureGates struct {
	Enabled []string `yaml:"enabled"`
}

// OcmNotifierConfigManager holds the OcmBaseUrl field
type OcmNotifierConfigManager struct {
	OcmBaseUrl string `yaml:"ocmBaseUrl"`
}

// IsValid returns a nil error when the OcmNotifierConfig is valid
func (cfg *OcmNotifierConfig) IsValid() error {
	if _, err := url.Parse(cfg.ConfigManager.OcmBaseUrl); err != nil {
		return fmt.Errorf("OCM Base URL is not a parseable URL")
	}
	return nil
}

// GetOCMBaseURL returns the OcmBaseUrl from the OcmNotifierConfig object
func (cfg *OcmNotifierConfig) GetOCMBaseURL() *url.URL {
	url, _ := url.Parse(cfg.ConfigManager.OcmBaseUrl)
	return url
}

// IsValid returns a nil error when the OcmFeatureConfig is valid
func (cfg *OcmFeatureConfig) IsValid() error {
	return nil
}

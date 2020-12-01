package ocm

import (
	"fmt"
	"net/url"
)

type OcmClientConfig struct {
	ConfigManager ConfigManager `yaml:"configManager"`
}

type ConfigManager struct {
	OcmBaseUrl string `yaml:"ocmBaseUrl"`
}

func (cfg *OcmClientConfig) IsValid() error {
	if _, err := url.Parse(cfg.ConfigManager.OcmBaseUrl); err != nil {
		return fmt.Errorf("OCM Base URL is not a parseable URL")
	}
	return nil
}

func (cfg *OcmClientConfig) GetOCMBaseURL() *url.URL {
	url, _ := url.Parse(cfg.ConfigManager.OcmBaseUrl)
	return url
}

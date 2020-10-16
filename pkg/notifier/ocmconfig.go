package notifier

import (
	"fmt"
	"net/url"
)

type OcmNotifierConfig struct {
	ConfigManager OcmNotifierConfigManager `yaml:"configManager"`
}

type OcmNotifierConfigManager struct {
	OcmBaseUrl string `yaml:"ocmBaseUrl"`
}

func (cfg *OcmNotifierConfig) IsValid() error {
	if _, err := url.Parse(cfg.ConfigManager.OcmBaseUrl); err != nil {
		return fmt.Errorf("OCM Base URL is not a parseable URL")
	}
	return nil
}

func (cfg *OcmNotifierConfig) GetOCMBaseURL() *url.URL {
	url, _ := url.Parse(cfg.ConfigManager.OcmBaseUrl)
	return url
}

package notifier

import "strings"

const (
	// OCM denotes OCM as the config manager source
	OCM ConfigManagerSource = "OCM"
	// LOCAL denotes a local config manager source
	LOCAL ConfigManagerSource = "LOCAL"
)

// ConfigManagerSource is a type that denotes the source of configuration management
type ConfigManagerSource string

// NotifierConfig is a type that provides a NotifierConfig
type NotifierConfig struct {
	ConfigManager NotifierConfigManager `yaml:"configManager"`
}

// NotifierConfigManager is a type that provides a notifier source
type NotifierConfigManager struct {
	Source string `yaml:"source"`
}

// IsValid returns no error if the notifier config is valid
func (cfg *NotifierConfig) IsValid() error {
	// the source can be missing. if it's not empty, validate it is a supported value
	if cfg.ConfigManager.Source == "" {
		return nil
	}

	switch strings.ToUpper(cfg.ConfigManager.Source) {
	case string(OCM):
		return nil
	case string(LOCAL):
		return nil
	default:
		return ErrNoNotifierConfigured
	}
}

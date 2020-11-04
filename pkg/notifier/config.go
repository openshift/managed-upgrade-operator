package notifier

const (
	OCM ConfigManagerSource = "OCM"
)

type ConfigManagerSource string

type NotifierConfig struct {
	ConfigManager NotifierConfigManager `yaml:"configManager"`
}

type NotifierConfigManager struct {
	Source string `yaml:"source"`
}

func (cfg *NotifierConfig) IsValid() error {
	// the source can be missing. if it's not empty, validate it is a supported value
	if cfg.ConfigManager.Source == "" {
		return nil
	}

	switch cfg.ConfigManager.Source {
	case string(OCM):
		return nil
	default:
		return ErrNoNotifierConfigured
	}
}



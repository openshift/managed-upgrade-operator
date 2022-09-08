package validation

// ValidationConfig holds fields that control version validation
type ValidationConfig struct {
	Validation validation `yaml:"validation"`
}

type validation struct {
	Cincinnati bool `yaml:"cincinnati"`
}

// IsValid returns a nil error when the UpgradeConfigManagerConfig is valid
func (cfg *ValidationConfig) IsValid() error {
	return nil
}

package upgradeconfig

import (
	"fmt"
	"time"
)

type config struct {
	UpgradeWindow upgradeWindow `yaml:"upgradeWindow"`
}

type upgradeWindow struct {
	TimeOut time.Duration `yaml:"timeOut"`
}

func (cfg *config) IsValid() error {
	if cfg.UpgradeWindow.TimeOut <= 0 {
		return fmt.Errorf("Config upgrade window time out is invalid")
	}

	return nil
}

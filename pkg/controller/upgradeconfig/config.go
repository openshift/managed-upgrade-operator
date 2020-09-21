package upgradeconfig

import (
	"fmt"
	"time"
)

type config struct {
	UpgradeWindow upgradeWindow `yaml:"upgradeWindow"`
}

type upgradeWindow struct {
	TimeOut int `yaml:"timeOut" default:"60"`
}

func (cfg *config) IsValid() error {
	if cfg.UpgradeWindow.TimeOut <= 0 {
		return fmt.Errorf("Config upgrade window time out is invalid")
	}

	return nil
}

func (cfg *config) GetUpgradeWindowTimeOutDuration() time.Duration {
	return time.Duration(cfg.UpgradeWindow.TimeOut) * time.Minute
}

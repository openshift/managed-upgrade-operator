package upgradeconfig

import (
	"fmt"
	"time"
)

type config struct {
	UpgradeWindow upgradeWindow `yaml:"upgradeWindow"`
}

type upgradeWindow struct {
	TimeOut      int `yaml:"timeOut" default:"120"`
	DelayTrigger int `yaml:"delayTrigger" default:"30"`
}

func (cfg *config) IsValid() error {
	if cfg.UpgradeWindow.TimeOut < 0 {
		return fmt.Errorf("Config upgrade window time out is invalid")
	}
	if cfg.UpgradeWindow.DelayTrigger < 0 {
		return fmt.Errorf("Config upgrade window delay trigger is invalid")
	}
	return nil
}

func (cfg *config) GetUpgradeWindowTimeOutDuration() time.Duration {
	return time.Duration(cfg.UpgradeWindow.TimeOut) * time.Minute
}

func (cfg *config) GetUpgradeWindowDelayTriggerDuration() time.Duration {
	return time.Duration(cfg.UpgradeWindow.DelayTrigger) * time.Minute
}

package upgraders

import (
	"fmt"
	"time"

	ac "github.com/openshift/managed-upgrade-operator/pkg/availabilitychecks"
	"github.com/openshift/managed-upgrade-operator/pkg/drain"
)

type upgraderConfig struct {
	Maintenance                    maintenanceConfig                 `yaml:"maintenance"`
	Scale                          scaleConfig                       `yaml:"scale"`
	NodeDrain                      drain.NodeDrain                   `yaml:"nodeDrain"`
	HealthCheck                    healthCheck                       `yaml:"healthCheck"`
	ExtDependencyAvailabilityCheck ac.ExtDependencyAvailabilityCheck `yaml:"extDependencyAvailabilityChecks"`
	UpgradeWindow                  upgradeWindow                     `yaml:"upgradeWindow"`
	Environment                    environment                       `yaml:"environment"`
	FeatureGate                    featureGate                       `yaml:"featureGate"`
}

type featureGate struct {
	Enabled []string `yaml:"enabled"`
}

func (cfg *upgraderConfig) IsFeatureEnabled(feature string) bool {
	if len(cfg.FeatureGate.Enabled) > 0 {
		for _, f := range cfg.FeatureGate.Enabled {
			if f == feature {
				return true
			}
		}
	}
	return false
}

type maintenanceConfig struct {
	ControlPlaneTime int           `yaml:"controlPlaneTime" default:"60"`
	IgnoredAlerts    ignoredAlerts `yaml:"ignoredAlerts"`
}

type ignoredAlerts struct {
	// Generally upgrades should not fire critical alerts but there are some critical alerts that will fire.
	// e.g. 'etcdMembersDown' happens as the masters drain/reboot and a master is offline but this is expected and will resolve.
	// This is a list of critical alerts that can be ignored while upgrading of controlplane occurs
	ControlPlaneCriticals []string `yaml:"controlPlaneCriticals"`
}

func (cfg *maintenanceConfig) IsValid() error {
	if cfg.ControlPlaneTime <= 0 {
		return fmt.Errorf("config maintenance controlPlaneTime out is invalid")
	}

	return nil
}

func (cfg *maintenanceConfig) GetControlPlaneDuration() time.Duration {
	return time.Duration(cfg.ControlPlaneTime) * time.Minute
}

func (cfg *scaleConfig) IsValid() error {
	if cfg.TimeOut <= 0 {
		return fmt.Errorf("config scale timeOut is invalid")
	}

	return nil
}

type upgradeWindow struct {
	TimeOut      int `yaml:"timeOut" default:"120"`
	DelayTrigger int `yaml:"delayTrigger" default:"30"`
}

func (cfg *upgradeWindow) GetUpgradeWindowTimeOutDuration() time.Duration {
	return time.Duration(cfg.TimeOut) * time.Minute
}

func (cfg *upgradeWindow) GetUpgradeDelayedTriggerDuration() time.Duration {
	return time.Duration(cfg.DelayTrigger) * time.Minute
}

type scaleConfig struct {
	TimeOut int `yaml:"timeOut" default:"30"`
}

type healthCheck struct {
	IgnoredCriticals  []string `yaml:"ignoredCriticals"`
	IgnoredNamespaces []string `yaml:"ignoredNamespaces"`
}

func (cfg *upgraderConfig) IsValid() error {
	if err := cfg.Maintenance.IsValid(); err != nil {
		return err
	}
	if cfg.NodeDrain.Timeout <= 0 {
		return fmt.Errorf("config nodeDrain timeOut is invalid")
	}
	if cfg.NodeDrain.ExpectedNodeDrainTime <= 0 {
		return fmt.Errorf("config nodeDrain expectedNodeDrainTime is invalid")
	}
	if cfg.UpgradeWindow.DelayTrigger < 0 {
		return fmt.Errorf("config upgrade window delay trigger is invalid")
	}
	if cfg.UpgradeWindow.TimeOut < 0 {
		return fmt.Errorf("config upgrade window time out is invalid")
	}
	if len(cfg.ExtDependencyAvailabilityCheck.HTTP.URLS) > 0 && cfg.ExtDependencyAvailabilityCheck.HTTP.Timeout <= 0 || cfg.ExtDependencyAvailabilityCheck.HTTP.Timeout > 60 {
		return fmt.Errorf("config HTTP timeout is invalid (Requires int between 1 - 60 inclusive)")
	}
	return nil
}

func (cfg *upgraderConfig) GetScaleDuration() time.Duration {
	return time.Duration(cfg.Scale.TimeOut) * time.Minute
}

type environment struct {
	Fedramp bool `yaml:"fedramp"`
}

func (cfg *environment) IsFedramp() bool {
	return cfg.Fedramp
}

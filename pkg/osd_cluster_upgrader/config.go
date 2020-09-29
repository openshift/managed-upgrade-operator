package osd_cluster_upgrader

import (
	"fmt"
	"time"

	"github.com/openshift/managed-upgrade-operator/pkg/drain"
)

type osdUpgradeConfig struct {
	Maintenance  maintenanceConfig `yaml:"maintenance"`
	Scale        scaleConfig       `yaml:"scale"`
	NodeDrain    drain.NodeDrain   `yaml:"nodeDrain"`
	HealthCheck  healthCheck       `yaml:"healthCheck"`
	Verification verification      `yaml:"verification"`
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
		return fmt.Errorf("Config maintenace controlPlaneTime out is invalid")
	}

	return nil
}

func (cfg *maintenanceConfig) GetControlPlaneDuration() time.Duration {
	return time.Duration(cfg.ControlPlaneTime) * time.Minute
}

type scaleConfig struct {
	TimeOut int `yaml:"timeOut" default:"30"`
}

type healthCheck struct {
	IgnoredCriticals []string `yaml:"ignoredCriticals"`
}

type verification struct {
	IgnoredNamespaces        []string `yaml:"ignoredNamespaces"`
	NamespacePrefixesToCheck []string `yaml:"namespacePrefixesToCheck"`
}

func (cfg *osdUpgradeConfig) IsValid() error {
	if err := cfg.Maintenance.IsValid(); err != nil {
		return err
	}
	if cfg.Scale.TimeOut <= 0 {
		return fmt.Errorf("Config scale timeOut is invalid")
	}
	if cfg.NodeDrain.Timeout <= 0 {
		return fmt.Errorf("Config nodeDrain timeOut is invalid")
	}
	if cfg.NodeDrain.ExpectedNodeDrainTime <= 0 {
		return fmt.Errorf("Config nodeDrain expectedNodeDrainTime is invalid")
	}

	return nil
}

func (cfg *osdUpgradeConfig) GetScaleDuration() time.Duration {
	return time.Duration(cfg.Scale.TimeOut) * time.Minute
}

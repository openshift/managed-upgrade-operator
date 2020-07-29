package osd_cluster_upgrader

import (
	"fmt"
	"time"
)

type osdUpgradeConfig struct {
	Maintenance maintenanceConfig `yaml:"maintenance"`
	Scale       scaleConfig       `yaml:"scale"`
	NodeDrain   nodeDrain         `yaml:"nodeDrain"`
}

type maintenanceConfig struct {
	ControlPlaneTime time.Duration `yaml:"controlPlaneTime"`
	WorkerNodeTime   time.Duration `yaml:"workerNodeTime"`
}

type scaleConfig struct {
	TimeOut time.Duration `yaml:"timeOut"`
}

type nodeDrain struct {
	TimeOut time.Duration `yaml:"timeOut"`
}

func (cfg *osdUpgradeConfig) IsValid() error {
	if cfg.Maintenance.ControlPlaneTime <= 0 {
		return fmt.Errorf("Config maintenace controlPlaneTime out is invalid")
	}
	if cfg.Maintenance.WorkerNodeTime <= 0 {
		return fmt.Errorf("Config maintenace workerNodeTime is invalid")
	}
	if cfg.Scale.TimeOut <= 0 {
		return fmt.Errorf("Config scale timeOut is invalid")
	}
	if cfg.NodeDrain.TimeOut <= 0 {
		return fmt.Errorf("Config nodeDrain timeOut is invalid")
	}

	return nil
}

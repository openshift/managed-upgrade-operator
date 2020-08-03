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
	ControlPlaneTime int `yaml:"controlPlaneTime"`
	WorkerNodeTime   int `yaml:"workerNodeTime"`
}

type scaleConfig struct {
	TimeOut int `yaml:"timeOut"`
}

type nodeDrain struct {
	TimeOut int `yaml:"timeOut"`
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

func (cfg *osdUpgradeConfig) GetControlPlaneDuration() time.Duration {
	return time.Duration(cfg.Maintenance.ControlPlaneTime) * time.Minute
}

func (cfg *osdUpgradeConfig) GetWorkerNodeDuration() time.Duration {
	return time.Duration(cfg.Maintenance.WorkerNodeTime) * time.Minute
}

func (cfg *osdUpgradeConfig) GetScaleDuration() time.Duration {
	return time.Duration(cfg.Scale.TimeOut) * time.Minute
}

func (cfg *osdUpgradeConfig) GetNodeDrainDuration() time.Duration {
	return time.Duration(cfg.NodeDrain.TimeOut) * time.Minute
}

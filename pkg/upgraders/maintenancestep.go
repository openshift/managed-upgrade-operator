package upgraders

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
)

// CreateControlPlaneMaintWindow creates the maintenance window for control plane
func (c *clusterUpgrader) CreateControlPlaneMaintWindow(ctx context.Context, logger logr.Logger) (bool, error) {
	endTime := time.Now().Add(c.config.Maintenance.GetControlPlaneDuration())
	err := c.maintenance.StartControlPlane(endTime, c.upgradeConfig.Spec.Desired.Version, c.config.Maintenance.IgnoredAlerts.ControlPlaneCriticals)
	if err != nil {
		return false, err
	}

	return true, nil
}

// RemoveControlPlaneMaintWindow removes the maintenance window for control plane
func (c *clusterUpgrader) RemoveControlPlaneMaintWindow(ctx context.Context, logger logr.Logger) (bool, error) {
	err := c.maintenance.EndControlPlane()
	if err != nil {
		return false, err
	}

	return true, nil
}

// CreateWorkerMaintWindow creates the maintenance window for workers
func (c *clusterUpgrader) CreateWorkerMaintWindow(ctx context.Context, logger logr.Logger) (bool, error) {
	upgradingResult, err := c.machinery.IsUpgrading(c.client, "worker")
	if err != nil {
		return false, err
	}

	// Depending on how long the Control Plane takes all workers may be already upgraded.
	if !upgradingResult.IsUpgrading {
		logger.Info(fmt.Sprintf("Worker nodes are already upgraded. Skipping worker maintenance for %s", c.upgradeConfig.Spec.Desired.Version))
		return true, nil
	}

	pendingWorkerCount := upgradingResult.MachineCount - upgradingResult.UpdatedCount
	if pendingWorkerCount < 1 {
		logger.Info("No worker node left for upgrading.")
		return true, nil
	}

	// We use the maximum of the PDB drain timeout and node drain timeout to compute a 'worst case' wait time
	pdbForceDrainTimeout := time.Duration(c.upgradeConfig.Spec.PDBForceDrainTimeout) * time.Minute
	nodeDrainTimeout := c.config.NodeDrain.GetTimeOutDuration()
	waitTimePeriod := time.Duration(pendingWorkerCount) * pdbForceDrainTimeout
	if pdbForceDrainTimeout < nodeDrainTimeout {
		waitTimePeriod = time.Duration(pendingWorkerCount) * nodeDrainTimeout
	}

	// Action time is the expected time taken to upgrade a worker node
	maintenanceDurationPerNode := c.config.NodeDrain.GetExpectedDrainDuration()
	actionTimePeriod := time.Duration(pendingWorkerCount) * maintenanceDurationPerNode

	// Our worker maintenance window is a combination of 'wait time' and 'action time'
	totalWorkerMaintenanceDuration := waitTimePeriod + actionTimePeriod

	endTime := time.Now().Add(totalWorkerMaintenanceDuration)
	logger.Info(fmt.Sprintf("Creating worker node maintenance for %d remaining nodes if no previous silence, ending at %v", pendingWorkerCount, endTime))
	err = c.maintenance.SetWorker(endTime, c.upgradeConfig.Spec.Desired.Version, pendingWorkerCount)
	if err != nil {
		return false, err
	}

	return true, nil
}

// RemoveMaintWindow removes all the maintenance windows we created during the upgrade
func (c *clusterUpgrader) RemoveMaintWindow(ctx context.Context, logger logr.Logger) (bool, error) {
	err := c.maintenance.EndWorker()
	if err != nil {
		return false, err
	}

	return true, nil
}

package upgraders

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/openshift/managed-upgrade-operator/pkg/notifier"
)

// AllWorkersUpgraded checks whether all the worker nodes are ready with new config
func (c *clusterUpgrader) AllWorkersUpgraded(ctx context.Context, logger logr.Logger) (bool, error) {
	upgradingResult, errUpgrade := c.machinery.IsUpgrading(c.client, "worker")
	if errUpgrade != nil {
		return false, errUpgrade
	}

	silenceActive, errSilence := c.maintenance.IsActive()
	if errSilence != nil {
		return false, errSilence
	}

	if upgradingResult.IsUpgrading {
		logger.Info(fmt.Sprintf("not all workers are upgraded, upgraded: %v, total: %v", upgradingResult.UpdatedCount, upgradingResult.MachineCount))
		if !silenceActive {
			logger.Info("Worker upgrade timeout.")
			c.metrics.UpdateMetricUpgradeWorkerTimeout(c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version)
		} else {
			c.metrics.ResetMetricUpgradeWorkerTimeout(c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version)
		}
		return false, nil
	}

	err := c.notifier.Notify(notifier.MuoStateWorkerPlaneUpgradeFinishedSL)
	if err != nil {
		logger.Error(err, "failed to notify worker plane upgrade completion")
		return false, err
	}

	c.metrics.ResetMetricUpgradeWorkerTimeout(c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version)
	return true, nil
}

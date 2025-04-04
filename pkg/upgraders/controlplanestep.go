package upgraders

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/notifier"
)

// CommenceUpgrade will update the clusterversion object to apply the desired version to trigger real OCP upgrade
func (c *clusterUpgrader) CommenceUpgrade(ctx context.Context, logger logr.Logger) (bool, error) {

	// We can reset the window breached metric if we're commencing
	c.metrics.UpdateMetricUpgradeWindowNotBreached(c.upgradeConfig.Name)

	upgradeCommenced, err := c.cvClient.HasUpgradeCommenced(c.upgradeConfig)
	if err != nil {
		return false, err
	}
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("Skipping upgrade step %s", upgradev1alpha1.CommenceUpgrade))
		return true, nil
	}

	err = c.notifier.Notify(notifier.MuoStateControlPlaneUpgradeStartedSL)
	if err != nil {
		return false, err
	}
	clusterid := c.cvClient.GetClusterId()
	c.metrics.UpdateMetricControlplaneUpgradeStartedTimestamp(clusterid, c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version, time.Now())

	isComplete, err := c.cvClient.EnsureDesiredConfig(c.upgradeConfig)
	if err != nil {
		logger.Info("clusterversion has not been updated to desired version, will retry on next reconcile")
		return false, err
	}

	return isComplete, nil
}

// ControlPlaneUpgraded checks whether control plane is upgraded. The ClusterVersion reports when cvo and master nodes are upgraded.
func (c *clusterUpgrader) ControlPlaneUpgraded(ctx context.Context, logger logr.Logger) (bool, error) {
	clusterVersion, err := c.cvClient.GetClusterVersion()
	if err != nil {
		return false, err
	}

	isCompleted := c.cvClient.HasUpgradeCompleted(clusterVersion, c.upgradeConfig)
	if isCompleted {
		err = c.notifier.Notify(notifier.MuoStateControlPlaneUpgradeFinishedSL)
		if err != nil {
			return false, err
		}
		c.metrics.ResetMetricUpgradeControlPlaneTimeout(c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version)
		clusterid := c.cvClient.GetClusterId()
		c.metrics.UpdateMetricControlplaneUpgradeCompletedTimestamp(clusterid, c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version, time.Now())
		c.metrics.UpdateMetricWorkernodeUpgradeStartedTimestamp(clusterid, c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version, time.Now())
		return true, nil
	}

	history := cv.GetHistory(clusterVersion, c.upgradeConfig.Spec.Desired.Version)
	var upgradeStartTime time.Time
	if history != nil && !history.StartedTime.IsZero() {
		upgradeStartTime = history.StartedTime.Time
	} else {
		upgradeStartTime, err = time.Parse(time.RFC3339, c.upgradeConfig.Spec.UpgradeAt)
		if err != nil {
			return false, err //error parsing time string
		}
	}

	upgradeTimeout := c.config.Maintenance.GetControlPlaneDuration()
	if !upgradeStartTime.IsZero() && time.Now().After(upgradeStartTime.Add(upgradeTimeout)) {
		logger.Info("Control plane upgrade timeout")
		c.metrics.UpdateMetricUpgradeControlPlaneTimeout(c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version)
	}

	return false, nil
}

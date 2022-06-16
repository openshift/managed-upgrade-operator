package upgraders

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
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

	isComplete, err := c.cvClient.EnsureDesiredConfig(c.upgradeConfig)
	if err != nil {
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
	history := cv.GetHistory(clusterVersion, c.upgradeConfig.Spec.Desired.Version)
	var upgradeStartTime time.Time
	var controlPlaneCompleteTime time.Time
	if history == nil {
		upgradeStartTime, err = time.Parse(time.RFC3339, c.upgradeConfig.Spec.UpgradeAt)
		if err != nil {
			return false, err //error parsing time string
		}
	} else {
		upgradeStartTime = history.StartedTime.Time
		if !history.CompletionTime.IsZero() {
			controlPlaneCompleteTime = history.CompletionTime.Time
		}
	}

	upgradeTimeout := c.config.Maintenance.GetControlPlaneDuration()
	if !upgradeStartTime.IsZero() && controlPlaneCompleteTime.IsZero() && time.Now().After(upgradeStartTime.Add(upgradeTimeout)) {
		logger.Info("Control plane upgrade timeout")
		c.metrics.UpdateMetricUpgradeControlPlaneTimeout(c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version)
	}

	if isCompleted {
		c.metrics.ResetMetricUpgradeControlPlaneTimeout(c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version)
		return true, nil
	}

	return false, nil
}

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
	desired := c.upgradeConfig.Spec.Desired
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("ClusterVersion is already set to Channel %s Version %s, skipping %s", desired.Channel, desired.Version, upgradev1alpha1.CommenceUpgrade))
		return true, nil
	}

	logger.Info(fmt.Sprintf("Setting ClusterVersion to Channel %s, version %s", desired.Channel, desired.Version))
	isComplete, err := c.cvClient.EnsureDesiredVersion(c.upgradeConfig)
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
	if history == nil {
		return false, err
	}

	upgradeStartTime := history.StartedTime
	controlPlaneCompleteTime := history.CompletionTime
	upgradeTimeout := c.config.Maintenance.GetControlPlaneDuration()
	if !upgradeStartTime.IsZero() && controlPlaneCompleteTime == nil && time.Now().After(upgradeStartTime.Add(upgradeTimeout)) {
		logger.Info("Control plane upgrade timeout")
		c.metrics.UpdateMetricUpgradeControlPlaneTimeout(c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version)
	}

	if isCompleted {
		c.metrics.ResetMetricUpgradeControlPlaneTimeout(c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version)
		return true, nil
	}

	return false, nil
}

package upgraders

import (
	"context"
	"fmt"
	"time"

	"github.com/blang/semver/v4"
	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
)

func (c *clusterUpgrader) IsUpgradeable(ctx context.Context, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := c.cvClient.HasUpgradeCommenced(c.upgradeConfig)
	if err != nil {
		return false, err
	}
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("Skipping upgrade step %s", upgradev1alpha1.IsClusterUpgradable))
		return true, nil
	}

	clusterVersion, err := c.cvClient.GetClusterVersion()
	if err != nil {
		return false, err
	}
	currentVersion, err := cv.GetCurrentVersion(clusterVersion)
	if err != nil {
		return false, err
	}
	parsedCurrentVersion, err := semver.Parse(currentVersion)
	if err != nil {
		return false, err
	}

	desiredVersion := c.upgradeConfig.Spec.Desired.Version
	parsedDesiredVersion, err := semver.Parse(desiredVersion)
	if err != nil {
		return false, err
	}

	// if the upgradeable is false then we need to check the current version with upgrade version for y-stream update
	upgradeTime, err := time.Parse(time.RFC3339, c.upgradeConfig.Spec.UpgradeAt)
	if err != nil {
		return false, fmt.Errorf("failed to parse spec.upgradeAt: %w", err)
	}
	upgradeTimeText := upgradeTime.UTC().Format(time.UnixDate)

	for _, condition := range clusterVersion.Status.Conditions {
		if condition.Type == configv1.OperatorUpgradeable && condition.Status == configv1.ConditionFalse && parsedDesiredVersion.Major >= parsedCurrentVersion.Major && parsedDesiredVersion.Minor > parsedCurrentVersion.Minor {
			return false, fmt.Errorf(
				"cluster upgrade to version %s on %s has been cancelled: %s: %s",
				desiredVersion,
				upgradeTimeText,
				condition.Reason,
				condition.Message,
			)
		}
	}

	return true, nil
}

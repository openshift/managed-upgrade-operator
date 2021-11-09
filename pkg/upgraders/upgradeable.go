package upgraders

import (
	"context"
	"fmt"

	"github.com/blang/semver"
	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
)

func (c *clusterUpgrader) IsUpgradeableVersion(ctx context.Context, logger logr.Logger) (bool, error) {
	v := &configv1.ClusterVersion{}
	currentVersion, err := cv.GetCurrentVersion(v)
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

	for _, condition := range v.Status.Conditions {
		if condition.Type == configv1.OperatorUpgradeable {
			if condition.Status == configv1.ConditionTrue {
				return true, nil
			}
			if condition.Status == configv1.ConditionFalse {
				// if the upgradeable is false then we need to check the current version with upgrade version for y-stream update
				if parsedDesiredVersion.Major >= parsedCurrentVersion.Major && parsedDesiredVersion.Minor > parsedCurrentVersion.Minor {
					return false, err
				}
				if parsedDesiredVersion.Major >= parsedCurrentVersion.Major && parsedDesiredVersion.Minor == parsedCurrentVersion.Minor {
					return true, nil
				}
			}
		}
	}
	return true, nil
}

func (c *clusterUpgrader) IsUpgradeable(ctx context.Context, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := c.cvClient.HasUpgradeCommenced(c.upgradeConfig)
	if err != nil {
		return false, err
	}
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("Skipping upgrade step %s", upgradev1alpha1.IsClusterUpgradable))
		return true, nil
	}

	output, err := c.IsUpgradeableVersion(ctx, logger)
	if err != nil || !output {
		return false, err
	}

	return true, nil
}

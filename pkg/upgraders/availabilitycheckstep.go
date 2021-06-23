package upgraders

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
)

// ExternalDependencyAvailabilityCheck validates that external dependencies of the upgrade are available.
func (c *clusterUpgrader) ExternalDependencyAvailabilityCheck(ctx context.Context, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := c.cvClient.HasUpgradeCommenced(c.upgradeConfig)
	if err != nil {
		return false, err
	}
	desired := c.upgradeConfig.Spec.Desired
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("ClusterVersion is already set to Channel %s Version %s, skipping %s", desired.Channel, desired.Version, upgradev1alpha1.ExtDepAvailabilityCheck))
		return true, nil
	}

	if len(c.availabilityCheckers) == 0 {
		logger.Info("No external dependencies configured for availability checks. Skipping.")
		return true, nil
	}

	for _, check := range c.availabilityCheckers {
		logger.Info(fmt.Sprintf("Checking availability check for %T", check))
		err := check.AvailabilityCheck()
		if err != nil {
			logger.Info(fmt.Sprintf("Failed availability check for %T", check))
			return false, err
		}
		logger.Info(fmt.Sprintf("Availability check complete for %T", check))
	}
	return true, nil
}

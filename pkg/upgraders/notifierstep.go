package upgraders

import (
	"context"

	"github.com/go-logr/logr"

	"github.com/openshift/managed-upgrade-operator/pkg/notifier"
)

// SendStartedNotification sends a notification on upgrade commencement
func (c *clusterUpgrader) SendStartedNotification(ctx context.Context, logger logr.Logger) (bool, error) {

	// No need to send started notifications if we're in the upgrading phase
	upgradeCommenced, err := c.cvClient.HasUpgradeCommenced(c.upgradeConfig)
	if err != nil {
		return false, err
	}
	if upgradeCommenced {
		return true, nil
	}

	err = c.notifier.Notify(notifier.MuoStateStarted)
	if err != nil {
		return false, err
	}
	return true, nil
}

// SendCompletedNotification sends a notification on upgrade completion
func (c *clusterUpgrader) SendCompletedNotification(ctx context.Context, logger logr.Logger) (bool, error) {
	err := c.notifier.Notify(notifier.MuoStateCompleted)
	if err != nil {
		return false, err
	}
	return true, nil
}

// SendScaleSkippedNotification sends a notification on Muo skip capacityreservation
func (c *clusterUpgrader) SendScaleSkippedNotification(ctx context.Context, logger logr.Logger) error {
	err := c.notifier.Notify(notifier.MuoStateScaleSkipped)
	if err != nil {
		return err
	}
	return nil
}

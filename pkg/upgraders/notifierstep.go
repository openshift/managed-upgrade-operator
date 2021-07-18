package upgraders

import (
	"context"
	"time"

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

// UpgradeDelayedCheck checks and sends a notification on a delay to upgrade commencement
func (c *clusterUpgrader) UpgradeDelayedCheck(ctx context.Context, logger logr.Logger) (bool, error) {

	// No need to send started notifications if we're in the upgrading phase
	upgradeCommenced, err := c.cvClient.HasUpgradeCommenced(c.upgradeConfig)
	if err != nil {
		return false, err
	}
	if upgradeCommenced {
		return true, nil
	}

	// Get the managed upgrade start time from the upgrade config history
	h := c.upgradeConfig.Status.History.GetHistory(c.upgradeConfig.Spec.Desired.Version)
	if h == nil {
		return false, nil
	}
	startTime := h.StartTime.Time

	delayTimeoutTrigger := c.config.UpgradeWindow.GetUpgradeDelayedTriggerDuration()
	// Send notification if the managed upgrade started but did not hit the controlplane upgrade phase in delayTimeoutTrigger minutes
	if !startTime.IsZero() && delayTimeoutTrigger > 0 && time.Now().After(startTime.Add(delayTimeoutTrigger)) {
		err := c.notifier.Notify(notifier.MuoStateDelayed)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}


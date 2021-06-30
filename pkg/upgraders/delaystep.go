package upgraders

import (
	"context"
	"time"

	"github.com/go-logr/logr"

	"github.com/openshift/managed-upgrade-operator/pkg/notifier"
)

// UpgradeDelayedCheck will raise a 'delayed' event if the cluster has not commenced
// upgrade within a configurable amount of time.
func (c *osdUpgrader) UpgradeDelayedCheck(ctx context.Context, logger logr.Logger) (bool, error) {

	upgradeCommenced, err := c.cvClient.HasUpgradeCommenced(c.upgradeConfig)
	if err != nil {
		return false, err
	}

	// No need to send delayed notifications if we're in the upgrading phase
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
		err := c.notifier.Notify(notifier.StateDelayed)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

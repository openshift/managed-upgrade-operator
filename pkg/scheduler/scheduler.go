package scheduler

import (
	"github.com/prometheus/common/log"
	"time"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
)

//go:generate mockgen -destination=mocks/mockScheduler.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/scheduler Scheduler
type Scheduler interface {
	IsReadyToUpgrade(*upgradev1alpha1.UpgradeConfig, metrics.Metrics, time.Duration) bool
}

type scheduler struct{}

func NewScheduler() Scheduler {
	return &scheduler{}
}

func (s *scheduler) IsReadyToUpgrade(upgradeConfig *upgradev1alpha1.UpgradeConfig, metricsClient metrics.Metrics, timeOut time.Duration) bool {
	if !upgradeConfig.Spec.Proceed {
		log.Info("Upgrade cannot proceed", "proceed", upgradeConfig.Spec.Proceed)
		return false
	}
	upgradeTime, err := time.Parse(time.RFC3339, upgradeConfig.Spec.UpgradeAt)
	if err != nil {
		log.Error(err, "failed to parse spec.upgradeAt", upgradeConfig.Spec.UpgradeAt)
		return false
	}
	now := time.Now()
	if now.After(upgradeTime) {
		// Is the current time within the allowable upgrade window
		if upgradeTime.Add(timeOut * time.Minute).After(now) {
			return true
		}
		// We are past the maximum allowed time to commence upgrading
		log.Error(nil, "field spec.upgradeAt cannot have backdated time")
		metricsClient.UpdateMetricUpgradeWindowBreached(upgradeConfig.Name)
	} else {
		// It hasn't reached the upgrade window yet
		metricsClient.UpdateMetricUpgradeWindowNotBreached(upgradeConfig.Name)
		pendingTime := upgradeTime.Sub(now)
		log.Info("Upgrade is scheduled after", "hours", int(pendingTime.Hours()))
	}

	return false
}

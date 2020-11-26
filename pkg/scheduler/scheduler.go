package scheduler

import (
	"github.com/prometheus/common/log"
	"time"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
)

//go:generate mockgen -destination=mocks/mockScheduler.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/scheduler Scheduler
type Scheduler interface {
	IsReadyToUpgrade(*upgradev1alpha1.UpgradeConfig, time.Duration) SchedulerResult
}

type scheduler struct{}

func NewScheduler() Scheduler {
	return &scheduler{}
}

type SchedulerResult struct {
	IsReady          bool
	IsBreached       bool
	TimeUntilUpgrade time.Duration
}

func (s *scheduler) IsReadyToUpgrade(upgradeConfig *upgradev1alpha1.UpgradeConfig, timeOut time.Duration) SchedulerResult {
	upgradeTime, err := time.Parse(time.RFC3339, upgradeConfig.Spec.UpgradeAt)
	if err != nil {
		log.Error(err, "failed to parse spec.upgradeAt", upgradeConfig.Spec.UpgradeAt)
		return SchedulerResult{IsReady: false, IsBreached: false, TimeUntilUpgrade: 0}
	}
	now := time.Now()
	if now.After(upgradeTime) {
		// Is the current time within the allowable upgrade window
		if upgradeTime.Add(timeOut).After(now) {
			return SchedulerResult{IsReady: true, IsBreached: false, TimeUntilUpgrade: 0}
		}

		return SchedulerResult{IsReady: true, IsBreached: true, TimeUntilUpgrade: 0}
	}

	// It hasn't reached the upgrade window yet
	pendingTime := upgradeTime.Sub(now)
	log.Infof("Upgrade is scheduled in %d hours %d mins", int(pendingTime.Hours()), int(pendingTime.Minutes())-(int(pendingTime.Hours())*60))
	return SchedulerResult{IsReady: false, IsBreached: false, TimeUntilUpgrade: pendingTime}
}

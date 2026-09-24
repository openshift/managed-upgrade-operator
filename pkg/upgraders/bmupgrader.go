package upgraders

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	ac "github.com/openshift/managed-upgrade-operator/pkg/availabilitychecks"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/drain"
	"github.com/openshift/managed-upgrade-operator/pkg/eventmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	"github.com/openshift/managed-upgrade-operator/pkg/maintenance"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/scaler"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradesteps"
)

// bmUpgrader is a cluster upgrader suitable for self-managed bare metal clusters.
// It inherits from the base clusterUpgrader.
type bmUpgrader struct {
	*clusterUpgrader
}

// NewBMUpgrader creates a new instance of a bmUpgrader
func NewBMUpgrader(c client.Client, cfm configmanager.ConfigManager, mc metrics.Metrics, notifier eventmanager.EventManager) (*bmUpgrader, error) {
	cfg := &upgraderConfig{}
	err := cfm.Into(cfg)
	if err != nil {
		return nil, err
	}

	m, err := maintenance.NewBuilder().NewClient(c)
	if err != nil {
		return nil, err
	}

	acs, err := ac.GetAvailabilityCheckers(&cfg.ExtDependencyAvailabilityCheck)
	if err != nil {
		return nil, err
	}

	bu := bmUpgrader{
		clusterUpgrader: &clusterUpgrader{
			client:               c,
			metrics:              mc,
			cvClient:             cv.NewCVClient(c),
			notifier:             notifier,
			config:               cfg,
			scaler:               scaler.NewScaler(),
			drainstrategyBuilder: drain.NewBuilder(),
			maintenance:          m,
			machinery:            machinery.NewMachinery(),
			availabilityCheckers: acs,
		},
	}

	bu.steps = bmUpgradeSteps(&bu)

	return &bu, nil
}

// bmUpgradeSteps returns the ordered upgrade steps for BM clusters.
// Capacity reservation (extra worker scaling) is intentionally omitted.
func bmUpgradeSteps(u *bmUpgrader) []upgradesteps.UpgradeStep {
	return []upgradesteps.UpgradeStep{
		upgradesteps.Action(string(upgradev1alpha1.SendStartedNotification), u.SendStartedNotification),
		upgradesteps.Action(string(upgradev1alpha1.IsClusterUpgradable), u.IsUpgradeable),
		upgradesteps.Action(string(upgradev1alpha1.UpgradePreHealthCheck), u.PreUpgradeHealthCheck),
		upgradesteps.Action(string(upgradev1alpha1.ExtDepAvailabilityCheck), u.ExternalDependencyAvailabilityCheck),
		upgradesteps.Action(string(upgradev1alpha1.ControlPlaneMaintWindow), u.CreateControlPlaneMaintWindow),
		upgradesteps.Action(string(upgradev1alpha1.CommenceUpgrade), u.CommenceUpgrade),
		upgradesteps.Action(string(upgradev1alpha1.ControlPlaneUpgraded), u.ControlPlaneUpgraded),
		upgradesteps.Action(string(upgradev1alpha1.RemoveControlPlaneMaintWindow), u.RemoveControlPlaneMaintWindow),
		upgradesteps.Action(string(upgradev1alpha1.WorkersMaintWindow), u.CreateWorkerMaintWindow),
		upgradesteps.Action(string(upgradev1alpha1.AllWorkerNodesUpgraded), u.AllWorkersUpgraded),
		upgradesteps.Action(string(upgradev1alpha1.RemoveMaintWindow), u.RemoveMaintWindow),
		upgradesteps.Action(string(upgradev1alpha1.PostClusterHealthCheck), u.PostUpgradeHealthCheck),
		upgradesteps.Action(string(upgradev1alpha1.SendCompletedNotification), u.SendCompletedNotification),
	}
}

// UpgradeCluster performs the upgrade of the cluster and returns an indication of the
// last-executed upgrade phase and any error associated with the phase execution.
func (u *bmUpgrader) UpgradeCluster(ctx context.Context, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (upgradev1alpha1.UpgradePhase, error) {
	u.upgradeConfig = upgradeConfig
	return u.runSteps(ctx, logger, u.steps)
}

// HealthCheck performs a pre-upgrade healthcheck when an upgrade is scheduled in advance mainly
// to highlight and notify of issues which could get fixed before the upgrade begins.
func (u *bmUpgrader) HealthCheck(ctx context.Context, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	u.upgradeConfig = upgradeConfig
	ok, err := u.PreUpgradeHealthCheck(ctx, logger)
	return ok, err
}

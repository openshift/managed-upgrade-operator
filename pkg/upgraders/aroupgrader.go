package upgraders

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
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

// aroUpgrader is a cluster upgrader suitable for ARO clusters.
// It inherits from the base clusterUpgrader.
type aroUpgrader struct {
	*clusterUpgrader
}

// NewAROUpgrader creates a new instance of an aroUpgrader
func NewAROUpgrader(c client.Client, cfm configmanager.ConfigManager, mc metrics.Metrics, notifier eventmanager.EventManager) (*aroUpgrader, error) {
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

	au := aroUpgrader{
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

	steps := []upgradesteps.UpgradeStep{
		upgradesteps.Action(string(upgradev1alpha1.SendStartedNotification), au.SendStartedNotification),
		upgradesteps.Action(string(upgradev1alpha1.UpgradePreHealthCheck), au.PreUpgradeHealthCheck),
		upgradesteps.Action(string(upgradev1alpha1.ExtDepAvailabilityCheck), au.ExternalDependencyAvailabilityCheck),
		upgradesteps.Action(string(upgradev1alpha1.UpgradeScaleUpExtraNodes), au.EnsureExtraUpgradeWorkers),
		upgradesteps.Action(string(upgradev1alpha1.ControlPlaneMaintWindow), au.CreateControlPlaneMaintWindow),
		upgradesteps.Action(string(upgradev1alpha1.CommenceUpgrade), au.CommenceUpgrade),
		upgradesteps.Action(string(upgradev1alpha1.ControlPlaneUpgraded), au.ControlPlaneUpgraded),
		upgradesteps.Action(string(upgradev1alpha1.RemoveControlPlaneMaintWindow), au.RemoveControlPlaneMaintWindow),
		upgradesteps.Action(string(upgradev1alpha1.WorkersMaintWindow), au.CreateWorkerMaintWindow),
		upgradesteps.Action(string(upgradev1alpha1.AllWorkerNodesUpgraded), au.AllWorkersUpgraded),
		upgradesteps.Action(string(upgradev1alpha1.RemoveExtraScaledNodes), au.RemoveExtraScaledNodes),
		upgradesteps.Action(string(upgradev1alpha1.RemoveMaintWindow), au.RemoveMaintWindow),
		upgradesteps.Action(string(upgradev1alpha1.PostClusterHealthCheck), au.PostUpgradeHealthCheck),
		upgradesteps.Action(string(upgradev1alpha1.SendCompletedNotification), au.SendCompletedNotification),
	}
	au.steps = steps

	return &au, nil
}

// UpgradeCluster performs the upgrade of the cluster and returns an indication of the
// last-executed upgrade phase, the success condition of the phase, and any error associated
// with the phase execution.
func (u *aroUpgrader) UpgradeCluster(ctx context.Context, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (upgradev1alpha1.UpgradePhase, *upgradev1alpha1.UpgradeCondition, error) {
	u.upgradeConfig = upgradeConfig
	return u.runSteps(ctx, logger, u.steps)
}

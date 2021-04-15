package aro

import (
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
)

var (
	steps                  UpgradeSteps
	aroUpgradeStepOrdering = []upgradev1alpha1.UpgradeConditionType{}
)

// Represents a named series of steps as part of an upgrade process
type UpgradeSteps map[upgradev1alpha1.UpgradeConditionType]UpgradeStep

// Represents an individual step in the upgrade process
type UpgradeStep func(client.Client, *aroUpgradeConfig, scaler.Scaler, drain.NodeDrainStrategyBuilder, metrics.Metrics, maintenance.Maintenance, cv.ClusterVersion, eventmanager.EventManager, *upgradev1alpha1.UpgradeConfig, machinery.Machinery, ac.AvailabilityCheckers, logr.Logger) (bool, error)

// Represents the order in which to undertake upgrade steps
type UpgradeStepOrdering []upgradev1alpha1.UpgradeConditionType

func NewClient(c client.Client, cfm configmanager.ConfigManager, mc metrics.Metrics, notifier eventmanager.EventManager) (*aroClusterUpgrader, error) {
	cfg := &aroUpgradeConfig{}
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

	steps = map[upgradev1alpha1.UpgradeConditionType]UpgradeStep{}

	return &aroClusterUpgrader{
		Steps:                steps,
		Ordering:             aroUpgradeStepOrdering,
		client:               c,
		maintenance:          m,
		metrics:              mc,
		scaler:               scaler.NewScaler(),
		drainstrategyBuilder: drain.NewBuilder(),
		cvClient:             cv.NewCVClient(c),
		cfg:                  cfg,
		machinery:            machinery.NewMachinery(),
		notifier:             notifier,
		availabilityCheckers: acs,
	}, nil
}

type aroClusterUpgrader struct {
	Steps                UpgradeSteps
	Ordering             UpgradeStepOrdering
	client               client.Client
	maintenance          maintenance.Maintenance
	metrics              metrics.Metrics
	scaler               scaler.Scaler
	drainstrategyBuilder drain.NodeDrainStrategyBuilder
	cvClient             cv.ClusterVersion
	cfg                  *aroUpgradeConfig
	machinery            machinery.Machinery
	notifier             eventmanager.EventManager
	availabilityCheckers ac.AvailabilityCheckers
}

// This triggers the ARO upgrade process.
// TODO: Right now it shows dummy message that upgrade is done. Actual implementation pending.
func (cu aroClusterUpgrader) UpgradeCluster(upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (upgradev1alpha1.UpgradePhase, *upgradev1alpha1.UpgradeCondition, error) {
	logger.Info("Upgrading ARO cluster")
	condition := &upgradev1alpha1.UpgradeCondition{
		Type:    "UpgradeSuccessful",
		Status:  "Upgrade is completed",
		Reason:  "ARO Upgrade",
		Message: "ARO Upgrade is done",
	}
	return upgradev1alpha1.UpgradePhaseUpgraded, condition, nil
}

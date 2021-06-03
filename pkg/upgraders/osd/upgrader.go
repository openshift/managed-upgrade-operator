package osd

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
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
	"github.com/openshift/managed-upgrade-operator/pkg/notifier"
	"github.com/openshift/managed-upgrade-operator/pkg/scaler"
)

var (
	steps                  UpgradeSteps
	osdUpgradeStepOrdering = []upgradev1alpha1.UpgradeConditionType{
		upgradev1alpha1.SendStartedNotification,
		upgradev1alpha1.UpgradeDelayedCheck,
		upgradev1alpha1.UpgradePreHealthCheck,
		upgradev1alpha1.ExtDepAvailabilityCheck,
		upgradev1alpha1.UpgradeScaleUpExtraNodes,
		upgradev1alpha1.ControlPlaneMaintWindow,
		upgradev1alpha1.CommenceUpgrade,
		upgradev1alpha1.ControlPlaneUpgraded,
		upgradev1alpha1.RemoveControlPlaneMaintWindow,
		upgradev1alpha1.WorkersMaintWindow,
		upgradev1alpha1.AllWorkerNodesUpgraded,
		upgradev1alpha1.RemoveExtraScaledNodes,
		upgradev1alpha1.RemoveMaintWindow,
		upgradev1alpha1.PostClusterHealthCheck,
		upgradev1alpha1.SendCompletedNotification,
	}
)

// UpgradeSteps represents a named series of steps as part of an upgrade process
type UpgradeSteps map[upgradev1alpha1.UpgradeConditionType]UpgradeStep

// UpgradeStep represents an individual step in the upgrade process
type UpgradeStep func(client.Client, *osdUpgradeConfig, scaler.Scaler, drain.NodeDrainStrategyBuilder, metrics.Metrics, maintenance.Maintenance, cv.ClusterVersion, eventmanager.EventManager, *upgradev1alpha1.UpgradeConfig, machinery.Machinery, ac.AvailabilityCheckers, logr.Logger) (bool, error)

// UpgradeStepOrdering represents the order in which to undertake upgrade steps
type UpgradeStepOrdering []upgradev1alpha1.UpgradeConditionType

// NewClient returns a new osdClusterUpgrader
func NewClient(c client.Client, cfm configmanager.ConfigManager, mc metrics.Metrics, notifier eventmanager.EventManager) (*osdClusterUpgrader, error) {
	cfg := &osdUpgradeConfig{}
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

	steps = map[upgradev1alpha1.UpgradeConditionType]UpgradeStep{
		upgradev1alpha1.SendStartedNotification:       SendStartedNotification,
		upgradev1alpha1.UpgradeDelayedCheck:           UpgradeDelayedCheck,
		upgradev1alpha1.UpgradePreHealthCheck:         PreClusterHealthCheck,
		upgradev1alpha1.ExtDepAvailabilityCheck:       ExternalDependencyAvailabilityCheck,
		upgradev1alpha1.UpgradeScaleUpExtraNodes:      EnsureExtraUpgradeWorkers,
		upgradev1alpha1.ControlPlaneMaintWindow:       CreateControlPlaneMaintWindow,
		upgradev1alpha1.CommenceUpgrade:               CommenceUpgrade,
		upgradev1alpha1.ControlPlaneUpgraded:          ControlPlaneUpgraded,
		upgradev1alpha1.RemoveControlPlaneMaintWindow: RemoveControlPlaneMaintWindow,
		upgradev1alpha1.WorkersMaintWindow:            CreateWorkerMaintWindow,
		upgradev1alpha1.AllWorkerNodesUpgraded:        AllWorkersUpgraded,
		upgradev1alpha1.RemoveExtraScaledNodes:        RemoveExtraScaledNodes,
		upgradev1alpha1.RemoveMaintWindow:             RemoveMaintWindow,
		upgradev1alpha1.PostClusterHealthCheck:        PostClusterHealthCheck,
		upgradev1alpha1.SendCompletedNotification:     SendCompletedNotification,
	}

	return &osdClusterUpgrader{
		Steps:                steps,
		Ordering:             osdUpgradeStepOrdering,
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

// An OSD cluster upgrader implementing the ClusterUpgrader interface
type osdClusterUpgrader struct {
	Steps                UpgradeSteps
	Ordering             UpgradeStepOrdering
	client               client.Client
	maintenance          maintenance.Maintenance
	metrics              metrics.Metrics
	scaler               scaler.Scaler
	drainstrategyBuilder drain.NodeDrainStrategyBuilder
	cvClient             cv.ClusterVersion
	cfg                  *osdUpgradeConfig
	machinery            machinery.Machinery
	notifier             eventmanager.EventManager
	availabilityCheckers ac.AvailabilityCheckers
}

// PreClusterHealthCheck performs cluster healthy check
func PreClusterHealthCheck(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := cvClient.HasUpgradeCommenced(upgradeConfig)
	if err != nil {
		return false, err
	}
	desired := upgradeConfig.Spec.Desired
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("ClusterVersion is already set to Channel %s Version %s, skipping %s", desired.Channel, desired.Version, upgradev1alpha1.UpgradePreHealthCheck))
		return true, nil
	}

	ok, err := performClusterHealthCheck(c, metricsClient, cvClient, cfg, logger)
	if err != nil || !ok {
		metricsClient.UpdateMetricClusterCheckFailed(upgradeConfig.Name)
		return false, err
	}

	metricsClient.UpdateMetricClusterCheckSucceeded(upgradeConfig.Name)
	return true, nil
}

// EnsureExtraUpgradeWorkers will scale up new workers to ensure customer capacity while upgrading.
func EnsureExtraUpgradeWorkers(c client.Client, cfg *osdUpgradeConfig, s scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {
	// Skip the step scale up worker node if capacity reservation is set to false
	if !upgradeConfig.Spec.CapacityReservation {
		logger.Info("Do not need to scale up extra node(s) since the capacity reservation is disabled")
		return true, nil
	}

	upgradeCommenced, err := cvClient.HasUpgradeCommenced(upgradeConfig)
	if err != nil {
		return false, err
	}
	desired := upgradeConfig.Spec.Desired
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("ClusterVersion is already set to Channel %s Version %s, skipping %s", desired.Channel, desired.Version, upgradev1alpha1.UpgradeScaleUpExtraNodes))
		return true, nil
	}

	isScaled, err := s.EnsureScaleUpNodes(c, cfg.GetScaleDuration(), logger)
	if err != nil {
		if scaler.IsScaleTimeOutError(err) {
			metricsClient.UpdateMetricScalingFailed(upgradeConfig.Name)
		}
		return false, err
	}

	if isScaled {
		metricsClient.UpdateMetricScalingSucceeded(upgradeConfig.Name)
	}

	return isScaled, nil
}

// ExternalDependencyAvailabilityCheck validates that external dependencies of the upgrade are available.
func ExternalDependencyAvailabilityCheck(c client.Client, cfg *osdUpgradeConfig, s scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := cvClient.HasUpgradeCommenced(upgradeConfig)
	if err != nil {
		return false, err
	}
	desired := upgradeConfig.Spec.Desired
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("ClusterVersion is already set to Channel %s Version %s, skipping %s", desired.Channel, desired.Version, upgradev1alpha1.ExtDepAvailabilityCheck))
		return true, nil
	}

	if len(availabilityCheckers) == 0 {
		logger.Info("No external dependencies configured for availability checks. Skipping.")
		return true, nil
	}

	for _, check := range availabilityCheckers {
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

// CommenceUpgrade will update the clusterversion object to apply the desired version to trigger real OCP upgrade
func CommenceUpgrade(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {

	// We can reset the window breached metric if we're commencing
	metricsClient.UpdateMetricUpgradeWindowNotBreached(upgradeConfig.Name)

	upgradeCommenced, err := cvClient.HasUpgradeCommenced(upgradeConfig)
	if err != nil {
		return false, err
	}
	desired := upgradeConfig.Spec.Desired
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("ClusterVersion is already set to Channel %s Version %s, skipping %s", desired.Channel, desired.Version, upgradev1alpha1.CommenceUpgrade))
		return true, nil
	}

	logger.Info(fmt.Sprintf("Setting ClusterVersion to Channel %s, version %s", desired.Channel, desired.Version))
	isComplete, err := cvClient.EnsureDesiredVersion(upgradeConfig)
	if err != nil {
		return false, err
	}

	return isComplete, nil
}

// CreateControlPlaneMaintWindow creates the maintenance window for control plane
func CreateControlPlaneMaintWindow(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {
	endTime := time.Now().Add(cfg.Maintenance.GetControlPlaneDuration())
	err := m.StartControlPlane(endTime, upgradeConfig.Spec.Desired.Version, cfg.Maintenance.IgnoredAlerts.ControlPlaneCriticals)
	if err != nil {
		return false, err
	}

	return true, nil
}

// RemoveControlPlaneMaintWindow removes the maintenance window for control plane
func RemoveControlPlaneMaintWindow(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {
	err := m.EndControlPlane()
	if err != nil {
		return false, err
	}

	return true, nil
}

// CreateWorkerMaintWindow creates the maintenance window for workers
func CreateWorkerMaintWindow(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {
	upgradingResult, err := machinery.IsUpgrading(c, "worker")
	if err != nil {
		return false, err
	}

	// Depending on how long the Control Plane takes all workers may be already upgraded.
	if !upgradingResult.IsUpgrading {
		logger.Info(fmt.Sprintf("Worker nodes are already upgraded. Skipping worker maintenance for %s", upgradeConfig.Spec.Desired.Version))
		return true, nil
	}

	pendingWorkerCount := upgradingResult.MachineCount - upgradingResult.UpdatedCount
	if pendingWorkerCount < 1 {
		logger.Info("No worker node left for upgrading.")
		return true, nil
	}

	// We use the maximum of the PDB drain timeout and node drain timeout to compute a 'worst case' wait time
	pdbForceDrainTimeout := time.Duration(upgradeConfig.Spec.PDBForceDrainTimeout) * time.Minute
	nodeDrainTimeout := cfg.NodeDrain.GetTimeOutDuration()
	waitTimePeriod := time.Duration(pendingWorkerCount) * pdbForceDrainTimeout
	if pdbForceDrainTimeout < nodeDrainTimeout {
		waitTimePeriod = time.Duration(pendingWorkerCount) * nodeDrainTimeout
	}

	// Action time is the expected time taken to upgrade a worker node
	maintenanceDurationPerNode := cfg.NodeDrain.GetExpectedDrainDuration()
	actionTimePeriod := time.Duration(pendingWorkerCount) * maintenanceDurationPerNode

	// Our worker maintenance window is a combination of 'wait time' and 'action time'
	totalWorkerMaintenanceDuration := waitTimePeriod + actionTimePeriod

	endTime := time.Now().Add(totalWorkerMaintenanceDuration)
	logger.Info(fmt.Sprintf("Creating worker node maintenance for %d remaining nodes if no previous silence, ending at %v", pendingWorkerCount, endTime))
	err = m.SetWorker(endTime, upgradeConfig.Spec.Desired.Version, pendingWorkerCount)
	if err != nil {
		return false, err
	}

	return true, nil
}

// AllWorkersUpgraded checks whether all the worker nodes are ready with new config
func AllWorkersUpgraded(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {
	upgradingResult, errUpgrade := machinery.IsUpgrading(c, "worker")
	if errUpgrade != nil {
		return false, errUpgrade
	}

	silenceActive, errSilence := m.IsActive()
	if errSilence != nil {
		return false, errSilence
	}

	if upgradingResult.IsUpgrading {
		logger.Info(fmt.Sprintf("not all workers are upgraded, upgraded: %v, total: %v", upgradingResult.UpdatedCount, upgradingResult.MachineCount))
		if !silenceActive {
			logger.Info("Worker upgrade timeout.")
			metricsClient.UpdateMetricUpgradeWorkerTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
		} else {
			metricsClient.ResetMetricUpgradeWorkerTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
		}
		return false, nil
	}

	metricsClient.ResetMetricUpgradeWorkerTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
	return true, nil
}

// RemoveExtraScaledNodes will scale down the extra workers added pre upgrade.
func RemoveExtraScaledNodes(c client.Client, cfg *osdUpgradeConfig, s scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {
	// Skip the step scale down worker node if capacity reservation is set to false
	if !upgradeConfig.Spec.CapacityReservation {
		logger.Info("Do not need to remove nodes since the capacity reservation is disabled")
		return true, nil
	}

	nds, err := dsb.NewNodeDrainStrategy(c, upgradeConfig, &cfg.NodeDrain)
	if err != nil {
		return false, err
	}
	isScaledDown, err := s.EnsureScaleDownNodes(c, nds, logger)
	if err != nil {
		dtErr, ok := scaler.IsDrainTimeOutError(err)
		if ok {
			metricsClient.UpdateMetricNodeDrainFailed(dtErr.GetNodeName())
		}
		logger.Error(err, "Extra upgrade node failed to drain in time")
		return false, err
	}

	if isScaledDown {
		metricsClient.ResetAllMetricNodeDrainFailed()
	}

	return isScaledDown, nil
}

// RemoveMaintWindow removes all the maintenance windows we created during the upgrade
func RemoveMaintWindow(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {
	err := m.EndWorker()
	if err != nil {
		return false, err
	}

	return true, nil
}

// PostClusterHealthCheck performs cluster health check after upgrade
func PostClusterHealthCheck(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {
	ok, err := performClusterHealthCheck(c, metricsClient, cvClient, cfg, logger)
	if err != nil || !ok {
		metricsClient.UpdateMetricClusterCheckFailed(upgradeConfig.Name)
		return false, err
	}

	metricsClient.UpdateMetricClusterCheckSucceeded(upgradeConfig.Name)
	return true, nil
}

// ControlPlaneUpgraded checks whether control plane is upgraded. The ClusterVersion reports when cvo and master nodes are upgraded.
func ControlPlaneUpgraded(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {
	clusterVersion, err := cvClient.GetClusterVersion()
	if err != nil {
		return false, err
	}

	isCompleted := cvClient.HasUpgradeCompleted(clusterVersion, upgradeConfig)
	history := cv.GetHistory(clusterVersion, upgradeConfig.Spec.Desired.Version)
	if history == nil {
		return false, err
	}

	upgradeStartTime := history.StartedTime
	controlPlaneCompleteTime := history.CompletionTime
	upgradeTimeout := cfg.Maintenance.GetControlPlaneDuration()
	if !upgradeStartTime.IsZero() && controlPlaneCompleteTime == nil && time.Now().After(upgradeStartTime.Add(upgradeTimeout)) {
		logger.Info("Control plane upgrade timeout")
		metricsClient.UpdateMetricUpgradeControlPlaneTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
	}

	if isCompleted {
		metricsClient.ResetMetricUpgradeControlPlaneTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
		return true, nil
	}

	return false, nil
}

// SendStartedNotification sends a notification on upgrade commencement
func SendStartedNotification(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {
	err := nc.Notify(notifier.StateStarted)
	if err != nil {
		return false, err
	}
	return true, nil
}

// UpgradeDelayedCheck checks and sends a notification on a delay to upgrade commencement
func UpgradeDelayedCheck(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {

	upgradeCommenced, err := cvClient.HasUpgradeCommenced(upgradeConfig)
	if err != nil {
		return false, err
	}

	// No need to send delayed notifications if we're in the upgrading phase
	if upgradeCommenced {
		return true, nil
	}

	// Get the managed upgrade start time from the upgrade config history
	h := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
	if h == nil {
		return false, nil
	}
	startTime := h.StartTime.Time

	delayTimeoutTrigger := cfg.UpgradeWindow.GetUpgradeDelayedTriggerDuration()
	// Send notification if the managed upgrade started but did not hit the controlplane upgrade phase in delayTimeoutTrigger minutes
	if !startTime.IsZero() && delayTimeoutTrigger > 0 && time.Now().After(startTime.Add(delayTimeoutTrigger)) {
		err := nc.Notify(notifier.StateDelayed)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

// SendCompletedNotification sends a notification on upgrade completion
func SendCompletedNotification(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, dsb drain.NodeDrainStrategyBuilder, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, availabilityCheckers ac.AvailabilityCheckers, logger logr.Logger) (bool, error) {
	err := nc.Notify(notifier.StateCompleted)
	if err != nil {
		return false, err
	}
	return true, nil
}

// Flags if the cluster has reached a condition during upgrade where it should be treated as failed
func shouldFailUpgrade(cvClient cv.ClusterVersion, cfg *osdUpgradeConfig, upgradeConfig *upgradev1alpha1.UpgradeConfig) (bool, error) {
	commenced, err := cvClient.HasUpgradeCommenced(upgradeConfig)
	if err != nil {
		return false, err
	}
	// If the upgrade has commenced, there's no going back
	if commenced {
		return false, nil
	}

	// Get the managed upgrade start time from upgrade config history
	h := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
	if h == nil {
		return false, nil
	}
	startTime := h.StartTime.Time

	upgradeWindowDuration := cfg.UpgradeWindow.GetUpgradeWindowTimeOutDuration()
	if !startTime.IsZero() && upgradeWindowDuration > 0 && time.Now().After(startTime.Add(upgradeWindowDuration)) {
		return true, nil
	}
	return false, nil
}

// This trigger the upgrade process
func (cu osdClusterUpgrader) UpgradeCluster(upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (upgradev1alpha1.UpgradePhase, *upgradev1alpha1.UpgradeCondition, error) {
	logger.Info("Upgrading cluster")

	// Determine if the upgrade has reached conditions warranting failure
	cancelUpgrade, _ := shouldFailUpgrade(cu.cvClient, cu.cfg, upgradeConfig)
	if cancelUpgrade {

		// Perform whatever actions are needed in the event of an upgrade failure
		err := performUpgradeFailure(cu.client, cu.metrics, cu.scaler, cu.notifier, upgradeConfig, logger)

		// If we couldn't notify of failure - do nothing, return the existing phase, try again next time
		if err != nil {
			h := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
			condition := newUpgradeCondition("Upgrade failed", "FailedUpgrade notification sent", "FailedUpgrade", corev1.ConditionFalse)
			return h.Phase, condition, nil
		}

		logger.Info("Failing upgrade")
		condition := newUpgradeCondition("Upgrade failed", "FailedUpgrade notification sent", "FailedUpgrade", corev1.ConditionTrue)
		return upgradev1alpha1.UpgradePhaseFailed, condition, nil
	}

	for _, key := range cu.Ordering {

		logger.Info(fmt.Sprintf("Performing %s", key))
		result, err := cu.Steps[key](cu.client, cu.cfg, cu.scaler, cu.drainstrategyBuilder, cu.metrics, cu.maintenance, cu.cvClient, cu.notifier, upgradeConfig, cu.machinery, cu.availabilityCheckers, logger)

		if err != nil {
			logger.Error(err, fmt.Sprintf("Error when %s", key))
			condition := newUpgradeCondition(fmt.Sprintf("%s not done", key), err.Error(), key, corev1.ConditionFalse)
			return upgradev1alpha1.UpgradePhaseUpgrading, condition, err
		}
		if !result {
			logger.Info(fmt.Sprintf("%s not done, skip following steps", key))
			condition := newUpgradeCondition(fmt.Sprintf("%s not done", key), fmt.Sprintf("%s still in progress", key), key, corev1.ConditionFalse)
			return upgradev1alpha1.UpgradePhaseUpgrading, condition, nil
		}
	}

	key := cu.Ordering[len(cu.Ordering)-1]
	condition := newUpgradeCondition(fmt.Sprintf("%s done", key), fmt.Sprintf("%s is completed", key), key, corev1.ConditionTrue)
	return upgradev1alpha1.UpgradePhaseUpgraded, condition, nil
}

// Carry out routines related to moving to an upgrade-failed state
func performUpgradeFailure(c client.Client, metricsClient metrics.Metrics, s scaler.Scaler, nc eventmanager.EventManager, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) error {
	// TearDown the extra machineset
	_, err := s.EnsureScaleDownNodes(c, nil, logger)
	if err != nil {
		logger.Error(err, "Failed to scale down the temporary upgrade machine when upgrade failed")
		return err
	}

	// Notify of failure
	err = nc.Notify(notifier.StateFailed)
	if err != nil {
		return err
	}

	// flag window breached metric
	metricsClient.UpdateMetricUpgradeWindowBreached(upgradeConfig.Name)

	// cancel previously triggered metrics
	metricsClient.ResetFailureMetrics()

	return nil
}

// check several things about the cluster and report problems
// * critical alerts
// * degraded operators (if there are critical alerts only)
func performClusterHealthCheck(c client.Client, metricsClient metrics.Metrics, cvClient cv.ClusterVersion, cfg *osdUpgradeConfig, logger logr.Logger) (bool, error) {
	ic := cfg.HealthCheck.IgnoredCriticals
	icQuery := ""
	if len(ic) > 0 {
		icQuery = `,alertname!="` + strings.Join(ic, `",alertname!="`) + `"`
	}
	healthCheckQuery := `ALERTS{alertstate="firing",severity="critical",namespace=~"^openshift.*|^kube-.*|^default$",namespace!="openshift-customer-monitoring",namespace!="openshift-logging",namespace!="openshift-operators"` + icQuery + "}"
	alerts, err := metricsClient.Query(healthCheckQuery)
	if err != nil {
		return false, fmt.Errorf("unable to query critical alerts: %s", err)
	}

	alertCount := len(alerts.Data.Result)

	if alertCount > 0 {
		alert := []string{}
		uniqueAlerts := make(map[string]bool)

		for _, r := range alerts.Data.Result {
			a := r.Metric["alertname"]

			if uniqueAlerts[a] {
				continue
			}
			alert = append(alert, a)
			uniqueAlerts[a] = true
		}

		logger.Info(fmt.Sprintf("Critical alert(s) firing: %s. Cannot continue upgrade", strings.Join(alert, ", ")))
		return false, fmt.Errorf("critical alert(s) firing: %s", strings.Join(alert, ", "))
	}

	result, err := cvClient.HasDegradedOperators()
	if err != nil {
		return false, err
	}
	if len(result.Degraded) > 0 {
		logger.Info(fmt.Sprintf("Degraded operators: %s", strings.Join(result.Degraded, ", ")))
		// Send the metrics for the cluster check failed if we have degraded operators
		return false, fmt.Errorf("degraded operators: %s", strings.Join(result.Degraded, ", "))
	}

	return true, nil
}

func newUpgradeCondition(reason, msg string, conditionType upgradev1alpha1.UpgradeConditionType, s corev1.ConditionStatus) *upgradev1alpha1.UpgradeCondition {
	return &upgradev1alpha1.UpgradeCondition{
		Type:    conditionType,
		Status:  s,
		Reason:  reason,
		Message: msg,
	}
}

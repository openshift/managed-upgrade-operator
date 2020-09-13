package osd_cluster_upgrader

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	operatorv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	"github.com/openshift/managed-upgrade-operator/pkg/maintenance"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/scaler"
)

var (
	steps                  UpgradeSteps
	osdUpgradeStepOrdering = []upgradev1alpha1.UpgradeConditionType{
		upgradev1alpha1.UpgradePreHealthCheck,
		upgradev1alpha1.UpgradeScaleUpExtraNodes,
		upgradev1alpha1.ControlPlaneMaintWindow,
		upgradev1alpha1.CommenceUpgrade,
		upgradev1alpha1.ControlPlaneUpgraded,
		upgradev1alpha1.RemoveControlPlaneMaintWindow,
		upgradev1alpha1.WorkersMaintWindow,
		upgradev1alpha1.AllWorkerNodesUpgraded,
		upgradev1alpha1.RemoveExtraScaledNodes,
		upgradev1alpha1.UpdateSubscriptions,
		upgradev1alpha1.PostUpgradeVerification,
		upgradev1alpha1.RemoveMaintWindow,
		upgradev1alpha1.PostClusterHealthCheck,
	}
)

// Represents a named series of steps as part of an upgrade process
type UpgradeSteps map[upgradev1alpha1.UpgradeConditionType]UpgradeStep

// Represents an individual step in the upgrade process
type UpgradeStep func(client.Client, *osdUpgradeConfig, scaler.Scaler, metrics.Metrics, maintenance.Maintenance, cv.ClusterVersion, *upgradev1alpha1.UpgradeConfig, machinery.Machinery, logr.Logger) (bool, error)

// Represents the order in which to undertake upgrade steps
type UpgradeStepOrdering []upgradev1alpha1.UpgradeConditionType

func NewClient(c client.Client, cfm configmanager.ConfigManager, mc metrics.Metrics) (*osdClusterUpgrader, error) {
	cfg := &osdUpgradeConfig{}
	err := cfm.Into(cfg)
	if err != nil {
		return nil, err
	}

	m, err := maintenance.NewBuilder().NewClient(c)
	if err != nil {
		return nil, err
	}

	steps = map[upgradev1alpha1.UpgradeConditionType]UpgradeStep{
		upgradev1alpha1.UpgradePreHealthCheck:         PreClusterHealthCheck,
		upgradev1alpha1.UpgradeScaleUpExtraNodes:      EnsureExtraUpgradeWorkers,
		upgradev1alpha1.ControlPlaneMaintWindow:       CreateControlPlaneMaintWindow,
		upgradev1alpha1.CommenceUpgrade:               CommenceUpgrade,
		upgradev1alpha1.ControlPlaneUpgraded:          ControlPlaneUpgraded,
		upgradev1alpha1.RemoveControlPlaneMaintWindow: RemoveControlPlaneMaintWindow,
		upgradev1alpha1.WorkersMaintWindow:            CreateWorkerMaintWindow,
		upgradev1alpha1.AllWorkerNodesUpgraded:        AllWorkersUpgraded,
		upgradev1alpha1.RemoveExtraScaledNodes:        RemoveExtraScaledNodes,
		upgradev1alpha1.UpdateSubscriptions:           UpdateSubscriptions,
		upgradev1alpha1.PostUpgradeVerification:       PostUpgradeVerification,
		upgradev1alpha1.RemoveMaintWindow:             RemoveMaintWindow,
		upgradev1alpha1.PostClusterHealthCheck:        PostClusterHealthCheck,
	}

	return &osdClusterUpgrader{
		Steps:       steps,
		Ordering:    osdUpgradeStepOrdering,
		client:      c,
		maintenance: m,
		metrics:     mc,
		scaler:      scaler.NewScaler(),
		cvClient:    cv.NewCVClient(c),
		cfg:         cfg,
		machinery:   machinery.NewMachinery(),
	}, nil
}

// An OSD cluster upgrader implementing the ClusterUpgrader interface
type osdClusterUpgrader struct {
	Steps       UpgradeSteps
	Ordering    UpgradeStepOrdering
	client      client.Client
	maintenance maintenance.Maintenance
	metrics     metrics.Metrics
	scaler      scaler.Scaler
	cvClient    cv.ClusterVersion
	cfg         *osdUpgradeConfig
	machinery   machinery.Machinery
}

// PreClusterHealthCheck performs cluster healthy check
func PreClusterHealthCheck(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := hasUpgradeCommenced(cvClient, upgradeConfig)
	if err != nil {
		return false, err
	}
	desired := upgradeConfig.Spec.Desired
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("ClusterVersion is already set to Channel %s Version %s, skipping %s", desired.Channel, desired.Version, upgradev1alpha1.UpgradePreHealthCheck))
		return true, nil
	}

	ok, err := performClusterHealthCheck(c, metricsClient, cfg, logger)
	if err != nil || !ok {
		metricsClient.UpdateMetricClusterCheckFailed(upgradeConfig.Name)
		return false, err
	}

	metricsClient.UpdateMetricClusterCheckSucceeded(upgradeConfig.Name)
	return true, nil
}

// EnsureExtraUpgradeWorkers will scale up new workers to ensure customer capacity while upgrading.
func EnsureExtraUpgradeWorkers(c client.Client, cfg *osdUpgradeConfig, s scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := hasUpgradeCommenced(cvClient, upgradeConfig)
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

// CommenceUpgrade will update the clusterversion object to apply the desired version to trigger real OCP upgrade
func CommenceUpgrade(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := hasUpgradeCommenced(cvClient, upgradeConfig)
	if err != nil {
		return false, err
	}
	desired := upgradeConfig.Spec.Desired
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("ClusterVersion is already set to Channel %s Version %s, skipping %s", desired.Channel, desired.Version, upgradev1alpha1.CommenceUpgrade))
		return true, nil
	}

	clusterVersion, err := cvClient.GetClusterVersion()
	if err != nil {
		return false, err
	}

	// Move the cluster to the same channel first
	if clusterVersion.Spec.Channel != desired.Channel {
		logger.Info(fmt.Sprintf("Moving cluster from Channel %s to Channel %s", clusterVersion.Spec.Channel, desired.Channel))
		clusterVersion.Spec.Channel = desired.Channel
		err = c.Update(context.TODO(), clusterVersion)
		// Always give a chance for re-reconcile and a CVO sync to occur
		return false, err
	}

	// The CVO may need time sync the version before launching the upgrade
	updateAvailable := false
	for _, update := range clusterVersion.Status.AvailableUpdates {
		if update.Version == desired.Version && update.Image != "" {
			updateAvailable = true
		}
	}
	if !updateAvailable {
		logger.Info(fmt.Sprintf("Waiting for CVO to sync Channel %s Version %s", desired.Channel, desired.Version))
		return false, nil
	}

	clusterVersion.Spec.Overrides = []configv1.ComponentOverride{}
	clusterVersion.Spec.DesiredUpdate = &configv1.Update{Version: upgradeConfig.Spec.Desired.Version}
	isSet, err := metricsClient.IsMetricUpgradeStartTimeSet(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
	if err != nil {
		return false, err
	}
	err = c.Update(context.TODO(), clusterVersion)
	if err != nil {
		return false, err
	}
	if !isSet {
		//Record the timestamp when we start the upgrade
		metricsClient.UpdateMetricUpgradeStartTime(time.Now(), upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
	}
	return true, nil
}

// CreateControlPlaneMaintWindow creates the maintenance window for control plane
func CreateControlPlaneMaintWindow(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, logger logr.Logger) (bool, error) {
	endTime := time.Now().Add(cfg.Maintenance.GetControlPlaneDuration())
	err := m.StartControlPlane(endTime, upgradeConfig.Spec.Desired.Version, cfg.Maintenance.IgnoredAlerts.ControlPlaneCriticals)
	if err != nil {
		return false, err
	}

	return true, nil
}

// RemoveControlPlaneMaintWindow removes the maintenance window for control plane
func RemoveControlPlaneMaintWindow(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, logger logr.Logger) (bool, error) {
	err := m.EndControlPlane()
	if err != nil {
		return false, err
	}

	return true, nil
}

// CreateWorkerMaintWindow creates the maintenance window for workers
func CreateWorkerMaintWindow(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, logger logr.Logger) (bool, error) {
	upgradingResult, err := machinery.IsUpgrading(c, "worker")
	if err != nil {
		return false, err
	}
	// Depending on how long the Control Plane takes all workers may be already upgraded.
	if !upgradingResult.IsUpgrading {
		logger.Info(fmt.Sprintf("Worker nodes are already upgraded. Skipping worker maintenace for %s", upgradeConfig.Spec.Desired.Version))
		return true, nil
	}

	pendingWorkerCount := upgradingResult.MachineCount - upgradingResult.UpdatedCount
	// We use the maximum of the PDB drain timeout and node drain timeout to compute a 'worst case' wait time
	pdbForceDrainTimeout := time.Duration(upgradeConfig.Spec.PDBForceDrainTimeout) * time.Minute
	nodeDrainTimeout := cfg.NodeDrain.GetDuration()
	waitTimePeriod := time.Duration(pendingWorkerCount) * pdbForceDrainTimeout
	if pdbForceDrainTimeout < nodeDrainTimeout {
		waitTimePeriod = time.Duration(pendingWorkerCount) * nodeDrainTimeout
	}

	// Action time is the expected time taken to upgrade a worker node
	maintenanceDurationPerNode := cfg.Maintenance.GetWorkerNodeDuration()
	actionTimePeriod := time.Duration(pendingWorkerCount) * maintenanceDurationPerNode

	// Our worker maintenance window is a combination of 'wait time' and 'action time'
	totalWorkerMaintenanceDuration := waitTimePeriod + actionTimePeriod

	endTime := time.Now().Add(totalWorkerMaintenanceDuration)
	logger.Info(fmt.Sprintf("Creating worker node maintenace for %d remaining nodes, ending at %v", pendingWorkerCount, endTime))
	err = m.SetWorker(endTime, upgradeConfig.Spec.Desired.Version)
	if err != nil {
		return false, err
	}

	return true, nil
}

// AllWorkersUpgraded checks whether all the worker nodes are ready with new config
func AllWorkersUpgraded(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, logger logr.Logger) (bool, error) {
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

	isSet, err := metricsClient.IsMetricNodeUpgradeEndTimeSet(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
	if err != nil {
		return false, err
	}
	if !isSet {
		metricsClient.UpdateMetricNodeUpgradeEndTime(time.Now(), upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
	}
	metricsClient.ResetMetricUpgradeWorkerTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
	return true, nil
}

// RemoveExtraScaledNodes will scale down the extra workers added pre upgrade.
func RemoveExtraScaledNodes(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, logger logr.Logger) (bool, error) {
	isScaled, err := scaler.EnsureScaleDownNodes(c, logger)
	if err != nil {
		logger.Error(err, "failed to get upgrade extra machinesets")
		return false, err
	}

	return isScaled, nil
}

// UpgradeSubscriptions will update the subscriptions for the 3rd party components, like logging
func UpdateSubscriptions(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, logger logr.Logger) (bool, error) {
	for _, item := range upgradeConfig.Spec.SubscriptionUpdates {
		sub := &operatorv1alpha1.Subscription{}
		err := c.Get(context.TODO(), types.NamespacedName{Namespace: item.Namespace, Name: item.Name}, sub)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("subscription :%s in namespace %s not exists, do not need update")
				continue
			} else {
				return false, err
			}
		}
		if sub.Spec.Channel != item.Channel {
			sub.Spec.Channel = item.Channel
			err = c.Update(context.TODO(), sub)
			if err != nil {
				return false, err
			}
		}
	}

	return true, nil
}

// PostUpgradeVerification run the verification steps which defined in performUpgradeVerification
func PostUpgradeVerification(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, logger logr.Logger) (bool, error) {
	ok, err := performUpgradeVerification(c, metricsClient, logger)
	if err != nil || !ok {
		metricsClient.UpdateMetricClusterVerificationFailed(upgradeConfig.Name)
		return false, err
	}

	metricsClient.UpdateMetricClusterVerificationSucceeded(upgradeConfig.Name)
	return true, nil
}

// performPostUpgradeVerification verifies all replicasets are at expected counts and all daemonsets are at expected counts
func performUpgradeVerification(c client.Client, metricsClient metrics.Metrics, logger logr.Logger) (bool, error) {

	namespacePrefixesToCheck := []string{"default", "kube", "openshift"}

	// Verify all ReplicaSets in the default, kube* and openshfit* namespaces are satisfied
	replicaSetList := &appsv1.ReplicaSetList{}
	err := c.List(context.TODO(), replicaSetList)
	if err != nil {
		return false, err
	}
	readyRs := 0
	totalRs := 0
	for _, replicaSet := range replicaSetList.Items {
		for _, namespacePrefix := range namespacePrefixesToCheck {
			if strings.HasPrefix(replicaSet.Namespace, namespacePrefix) {
				totalRs = totalRs + 1
				if replicaSet.Status.ReadyReplicas == replicaSet.Status.Replicas {
					readyRs = readyRs + 1
				}
			}
		}
	}
	if totalRs != readyRs {
		logger.Info(fmt.Sprintf("not all replicaset are ready:expected number :%v , ready number %v", totalRs, readyRs))
		return false, nil
	}

	// Verify all Daemonsets in the default, kube* and openshift* namespaces are satisfied
	daemonSetList := &appsv1.DaemonSetList{}
	err = c.List(context.TODO(), daemonSetList)
	if err != nil {
		return false, err
	}
	readyDS := 0
	totalDS := 0
	for _, daemonSet := range daemonSetList.Items {
		for _, namespacePrefix := range namespacePrefixesToCheck {
			if strings.HasPrefix(daemonSet.Namespace, namespacePrefix) {
				totalDS = totalDS + 1
				if daemonSet.Status.DesiredNumberScheduled == daemonSet.Status.NumberReady {
					readyDS = readyDS + 1
				}
			}
		}
	}
	if totalDS != readyDS {
		logger.Info(fmt.Sprintf("not all daemonset are ready:expected number :%v , ready number %v", totalDS, readyDS))
		return false, nil
	}

	// If daemonsets and replicasets are satisfied, any active TargetDown alerts will eventually go away.
	// Wait for that to occur before declaring the verification complete.
	namespacePrefixesAsRegex := make([]string, 0)
	for _, namespacePrefix := range namespacePrefixesToCheck {
		namespacePrefixesAsRegex = append(namespacePrefixesAsRegex, fmt.Sprintf("^%s-.*", namespacePrefix))
	}
	isTargetDownFiring, err := metricsClient.IsAlertFiring("TargetDown", namespacePrefixesAsRegex)
	if err != nil {
		return false, fmt.Errorf("can't query for alerts: %v", err)
	}
	if isTargetDownFiring {
		logger.Info(fmt.Sprintf("TargetDown alerts are still firing in namespaces %v", namespacePrefixesAsRegex))
		return false, nil
	}

	return true, nil
}

// RemoveMaintWindows removes all the maintenance windows we created during the upgrade
func RemoveMaintWindow(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, logger logr.Logger) (bool, error) {
	err := m.EndWorker()
	if err != nil {
		return false, err
	}

	return true, nil
}

// PostClusterHealthCheck performs cluster health check after upgrade
func PostClusterHealthCheck(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, logger logr.Logger) (bool, error) {
	ok, err := performClusterHealthCheck(c, metricsClient, cfg, logger)
	if err != nil || !ok {
		metricsClient.UpdateMetricClusterCheckFailed(upgradeConfig.Name)
		return false, err
	}

	metricsClient.UpdateMetricClusterCheckSucceeded(upgradeConfig.Name)
	return true, nil
}

// ControlPlaneUpgraded checks whether control plane is upgraded. The ClusterVersion reports when cvo and master nodes are upgraded.
func ControlPlaneUpgraded(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, cvClient cv.ClusterVersion, upgradeConfig *upgradev1alpha1.UpgradeConfig, machinery machinery.Machinery, logger logr.Logger) (bool, error) {
	clusterVersion, err := cvClient.GetClusterVersion()
	if err != nil {
		return false, err
	}

	isCompleted := false
	var upgradeStartTime metav1.Time
	var controlPlaneCompleteTime *metav1.Time
	for _, c := range clusterVersion.Status.History {
		if c.Version == upgradeConfig.Spec.Desired.Version {
			upgradeStartTime = c.StartedTime
			if c.State == configv1.CompletedUpdate {
				isCompleted = true
				controlPlaneCompleteTime = c.CompletionTime
			}
		}
	}

	upgradeTimeout := cfg.Maintenance.GetControlPlaneDuration()
	if !upgradeStartTime.IsZero() && controlPlaneCompleteTime == nil && time.Now().After(upgradeStartTime.Add(upgradeTimeout)) {
		logger.Info("Control plane upgrade timeout")
		metricsClient.UpdateMetricUpgradeControlPlaneTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
	}

	if isCompleted {
		isSet, err := metricsClient.IsMetricControlPlaneEndTimeSet(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
		if err != nil {
			return false, err
		}
		if !isSet {
			metricsClient.UpdateMetricControlPlaneEndTime(time.Now(), upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
		}
		metricsClient.ResetMetricUpgradeControlPlaneTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
		return true, nil
	}

	return false, nil
}

// This trigger the upgrade process
func (cu osdClusterUpgrader) UpgradeCluster(upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (upgradev1alpha1.UpgradePhase, *upgradev1alpha1.UpgradeCondition, error) {
	logger.Info("Upgrading cluster")

	for _, key := range cu.Ordering {

		logger.Info(fmt.Sprintf("Performing %s", key))

		result, err := cu.Steps[key](cu.client, cu.cfg, cu.scaler, cu.metrics, cu.maintenance, cu.cvClient, upgradeConfig, cu.machinery, logger)

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

// check several things about the cluster and report problems
// * critical alerts
// * degraded operators (if there are critical alerts only)
func performClusterHealthCheck(c client.Client, metricsClient metrics.Metrics, cfg *osdUpgradeConfig, logger logr.Logger) (bool, error) {
	ic := cfg.HealthCheck.IgnoredCriticals
	icQuery := ""
	if len(ic) > 0 {
		icQuery = `,alertname!="` + strings.Join(ic, `",alertname!="`) + `"`
	}
	healthCheckQuery := `ALERTS{alertstate="firing",severity="critical",namespace=~"^openshift.*|^kube.*|^default$",namespace!="openshift-customer-monitoring"` + icQuery + "}"
	alerts, err := metricsClient.Query(healthCheckQuery)
	if err != nil {
		return false, fmt.Errorf("Unable to query critical alerts: %s", err)
	}

	if len(alerts.Data.Result) > 0 {
		logger.Info("There are critical alerts exists, cannot upgrade now")
		return false, fmt.Errorf("There are %d critical alerts", len(alerts.Data.Result))
	}

	//check co status

	operatorList := &configv1.ClusterOperatorList{}
	err = c.List(context.TODO(), operatorList, []client.ListOption{}...)
	if err != nil {
		return false, err
	}

	degradedOperators := []string{}
	for _, co := range operatorList.Items {
		for _, condition := range co.Status.Conditions {
			if (condition.Type == configv1.OperatorDegraded && condition.Status == configv1.ConditionTrue) || (condition.Type == configv1.OperatorAvailable && condition.Status == configv1.ConditionFalse) {
				degradedOperators = append(degradedOperators, co.Name)
			}
		}
	}

	if len(degradedOperators) > 0 {
		logger.Info(fmt.Sprintf("degraded operators :%s", strings.Join(degradedOperators, ",")))
		// Send the metrics for the cluster check failed if we have degraded operators
		return false, fmt.Errorf("degraded operators :%s", strings.Join(degradedOperators, ","))
	}
	return true, nil

}

func GetCurrentVersion(clusterVersion *configv1.ClusterVersion) (string, error) {
	var gotVersion string
	var latestCompletionTime *metav1.Time = nil
	for _, history := range clusterVersion.Status.History {
		if history.State == configv1.CompletedUpdate {
			if latestCompletionTime == nil || history.CompletionTime.After(latestCompletionTime.Time) {
				gotVersion = history.Version
				latestCompletionTime = history.CompletionTime
			}
		}
	}

	if len(gotVersion) == 0 {
		return gotVersion, fmt.Errorf("Failed to get current version")
	}

	return gotVersion, nil
}

func newUpgradeCondition(reason, msg string, conditionType upgradev1alpha1.UpgradeConditionType, s corev1.ConditionStatus) *upgradev1alpha1.UpgradeCondition {
	return &upgradev1alpha1.UpgradeCondition{
		Type:    conditionType,
		Status:  s,
		Reason:  reason,
		Message: msg,
	}
}

func isEqualVersion(cv *configv1.ClusterVersion, uc *upgradev1alpha1.UpgradeConfig) bool {
	if cv.Spec.DesiredUpdate != nil &&
		cv.Spec.DesiredUpdate.Version == uc.Spec.Desired.Version &&
		cv.Spec.Channel == uc.Spec.Desired.Channel {
		return true
	}

	return false
}

// hasUpgradeCommenced checks if the upgrade has commenced
func hasUpgradeCommenced(cvClient cv.ClusterVersion, uc *upgradev1alpha1.UpgradeConfig) (bool, error) {
	clusterVersion, err := cvClient.GetClusterVersion()
	if err != nil {
		return false, err
	}

	if !isEqualVersion(clusterVersion, uc) {
		return false, nil
	}

	return true, nil
}


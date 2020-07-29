package osd_cluster_upgrader

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	operatorv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/maintenance"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/scaler"
)

var (
	once                   sync.Once
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
type UpgradeStep func(client.Client, *osdUpgradeConfig, scaler.Scaler, metrics.Metrics, maintenance.Maintenance, *upgradev1alpha1.UpgradeConfig, logr.Logger) (bool, error)

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

	once.Do(func() {
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
	})
	return &osdClusterUpgrader{
		Steps:       steps,
		Ordering:    osdUpgradeStepOrdering,
		client:      c,
		maintenance: m,
		metrics:     mc,
		scaler:      scaler.NewScaler(),
		cfg:         cfg,
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
	cfg         *osdUpgradeConfig
}

// PreClusterHealthCheck performs cluster healthy check
func PreClusterHealthCheck(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := hasUpgradeCommenced(c, upgradeConfig)
	if err != nil {
		return false, err
	}
	desired := upgradeConfig.Spec.Desired
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("ClusterVersion is already set to Channel %s Version %s, skipping %s", desired.Channel, desired.Version, upgradev1alpha1.UpgradePreHealthCheck))
		return true, nil
	}

	ok, err := performClusterHealthCheck(c, metricsClient, logger)
	if err != nil || !ok {
		metricsClient.UpdateMetricClusterCheckFailed(upgradeConfig.Name)
		return false, err
	}

	metricsClient.UpdateMetricClusterCheckSucceeded(upgradeConfig.Name)
	return true, nil
}

// EnsureExtraUpgradeWorkers will scale up new workers to ensure customer capacity while upgrading.
func EnsureExtraUpgradeWorkers(c client.Client, cfg *osdUpgradeConfig, s scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := hasUpgradeCommenced(c, upgradeConfig)
	if err != nil {
		return false, err
	}
	desired := upgradeConfig.Spec.Desired
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("ClusterVersion is already set to Channel %s Version %s, skipping %s", desired.Channel, desired.Version, upgradev1alpha1.UpgradeScaleUpExtraNodes))
		return true, nil
	}

	isScaled, err := s.EnsureScaleUpNodes(c, cfg.Scale.TimeOut, logger)
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
func CommenceUpgrade(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := hasUpgradeCommenced(c, upgradeConfig)
	if err != nil {
		return false, err
	}
	desired := upgradeConfig.Spec.Desired
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("ClusterVersion is already set to Channel %s Version %s, skipping %s", desired.Channel, desired.Version, upgradev1alpha1.CommenceUpgrade))
		return true, nil
	}

	cv, err := GetClusterVersion(c)
	if err != nil {
		return false, err
	}
	cv.Spec.Overrides = []configv1.ComponentOverride{}
	cv.Spec.DesiredUpdate = &configv1.Update{Version: upgradeConfig.Spec.Desired.Version}
	cv.Spec.Channel = upgradeConfig.Spec.Desired.Channel

	isSet, err := metricsClient.IsMetricUpgradeStartTimeSet(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
	if err != nil {
		return false, err
	}
	err = c.Update(context.TODO(), cv)
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
func CreateControlPlaneMaintWindow(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	endTime := time.Now().Add(cfg.Maintenance.ControlPlaneTime * time.Minute)
	err := m.StartControlPlane(endTime, upgradeConfig.Spec.Desired.Version)
	if err != nil {
		return false, err
	}

	return true, nil
}

// RemoveControlPlaneMaintWindow removes the maintenance window for control plane
func RemoveControlPlaneMaintWindow(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	err := m.End()
	if err != nil {
		return false, err
	}

	return true, nil
}

// CreateWorkerMaintWindow creates the maintenance window for workers
func CreateWorkerMaintWindow(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	configPool := &machineconfigapi.MachineConfigPool{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: "worker"}, configPool)
	if err != nil {
		return false, nil
	}

	// Depending on how long the Control Plane takes all workers may be already upgraded.
	pendingWorkerCount := configPool.Status.MachineCount - configPool.Status.UpdatedMachineCount
	if pendingWorkerCount == 0 {
		logger.Info(fmt.Sprintf("Worker nodes are already upgraded. Skipping worker maintenace for %s", upgradeConfig.Spec.Desired.Version))
		return true, nil
	}

	maintenanceDurationPerNode := cfg.Maintenance.WorkerNodeTime * time.Minute
	workerMaintenanceExpectedDuration := time.Duration(pendingWorkerCount) * maintenanceDurationPerNode
	endTime := time.Now().Add(workerMaintenanceExpectedDuration)
	err = m.StartWorker(endTime, upgradeConfig.Spec.Desired.Version)
	if err != nil {
		return false, err
	}

	return true, nil
}

// AllWorkersUpgraded checks whether all the worker nodes are ready with new config
func AllWorkersUpgraded(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	okDrain, errDrain := nodeDrained(c, cfg.NodeDrain.TimeOut, upgradeConfig, logger)
	if errDrain != nil {
		return false, errDrain
	}

	if !okDrain {
		logger.Info("Node drain timeout.")
		metricsClient.UpdateMetricNodeDrainFailed(upgradeConfig.Name)
		return false, nil
	}

	okUpgrade, errUpgrade := nodesUpgraded(c, "worker", logger)
	if errUpgrade != nil {
		return false, errUpgrade
	}

	silenceActive, errSilence := m.IsActive()
	if errSilence != nil {
		return false, errSilence
	}

	if !okUpgrade {
		if !silenceActive {
			logger.Info("Worker upgrade timeout.")
			metricsClient.UpdateMetricUpgradeWorkerTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version)
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
func RemoveExtraScaledNodes(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	isScaled, err := scaler.EnsureScaleDownNodes(c, logger)
	if err != nil {
		logger.Error(err, "failed to get upgrade extra machinesets")
		return false, err
	}

	return isScaled, nil
}

// UpgradeSubscriptions will update the subscriptions for the 3rd party components, like logging
func UpdateSubscriptions(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
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
func PostUpgradeVerification(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	ok, err := performUpgradeVerification(c, logger)
	if err != nil || !ok {
		metricsClient.UpdateMetricClusterVerificationFailed(upgradeConfig.Name)
		return false, err
	}

	metricsClient.UpdateMetricClusterVerificationSucceeded(upgradeConfig.Name)
	return true, nil
}

// performPostUpgradeVerification verifies all replicasets are at expected counts and all daemonsets are at expected counts
func performUpgradeVerification(c client.Client, logger logr.Logger) (bool, error) {
	replicaSetList := &appsv1.ReplicaSetList{}
	err := c.List(context.TODO(), replicaSetList)
	if err != nil {
		return false, err
	}
	readyRs := 0
	totalRs := 0
	for _, replica := range replicaSetList.Items {
		if strings.HasPrefix(replica.Namespace, "default") ||
			strings.HasPrefix(replica.Namespace, "kube") ||
			strings.HasPrefix(replica.Namespace, "openshift") {
			totalRs = totalRs + 1
			if replica.Status.ReadyReplicas == replica.Status.Replicas {
				readyRs = readyRs + 1
			}

		}
	}

	if totalRs != readyRs {
		logger.Info(fmt.Sprintf("not all replicaset are ready:expected number :%v , ready number %v", len(replicaSetList.Items), readyRs))
		return false, nil
	}

	dsList := &appsv1.DaemonSetList{}
	err = c.List(context.TODO(), dsList)
	if err != nil {
		return false, err
	}
	readyDS := 0
	totalDS := 0
	for _, ds := range dsList.Items {
		if strings.HasPrefix(ds.Namespace, "default") ||
			strings.HasPrefix(ds.Namespace, "kube") ||
			strings.HasPrefix(ds.Namespace, "openshift") {
			totalDS = totalDS + 1
			if ds.Status.DesiredNumberScheduled == ds.Status.NumberReady {
				readyDS = readyDS + 1
			}
		}
	}
	if totalDS != readyDS {
		logger.Info(fmt.Sprintf("not all daemonset are ready:expected number :%v , ready number %v", len(dsList.Items), readyDS))
		return false, nil
	}

	return true, nil
}

// RemoveMaintWindows removes all the maintenance windows we created during the upgrade
func RemoveMaintWindow(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	err := m.End()
	if err != nil {
		return false, err
	}

	return true, nil
}

// PostClusterHealthCheck performs cluster health check after upgrade
func PostClusterHealthCheck(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	ok, err := performClusterHealthCheck(c, metricsClient, logger)
	if err != nil || !ok {
		metricsClient.UpdateMetricClusterCheckFailed(upgradeConfig.Name)
		return false, err
	}

	metricsClient.UpdateMetricClusterCheckSucceeded(upgradeConfig.Name)
	return true, nil
}

// nodesUpgraded checks whether nodes are upgraded
func nodesUpgraded(c client.Client, nodeType string, logger logr.Logger) (bool, error) {
	configPool := &machineconfigapi.MachineConfigPool{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: nodeType}, configPool)
	if err != nil {
		return false, nil
	}
	if configPool.Status.MachineCount != configPool.Status.UpdatedMachineCount {
		errMsg := fmt.Sprintf("not all %s are upgraded, upgraded: %v, total: %v", nodeType, configPool.Status.UpdatedMachineCount, configPool.Status.MachineCount)
		logger.Info(errMsg)
		return false, nil
	}

	// send node upgrade complete metrics
	return true, nil
}

// ControlPlaneUpgraded checks whether control plane is upgraded. The ClusterVersion reports when cvo and master nodes are upgraded.
func ControlPlaneUpgraded(c client.Client, cfg *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	clusterVersion, err := GetClusterVersion(c)
	if err != nil {
		return false, err
	}

	isCompleted := false
	var upgradeStartTime metav1.Time
	var controlPlaneCompleteTime *metav1.Time
	for _, c := range clusterVersion.Status.History {
		if c.State == configv1.CompletedUpdate && c.Version == upgradeConfig.Spec.Desired.Version {
			isCompleted = true
			upgradeStartTime = c.StartedTime
			controlPlaneCompleteTime = c.CompletionTime
		}
	}

	upgradeTimeout := cfg.Maintenance.ControlPlaneTime * time.Minute
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

		result, err := cu.Steps[key](cu.client, cu.cfg, cu.scaler, cu.metrics, cu.maintenance, upgradeConfig, logger)

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
func performClusterHealthCheck(c client.Client, metricsClient metrics.Metrics, logger logr.Logger) (bool, error) {
	alerts, err := metricsClient.Query("ALERTS{alertstate=\"firing\",severity=\"critical\",namespace=~\"^openshift.*|^kube.*|^default$\",namespace!=\"openshift-customer-monitoring\",alertname!=\"ClusterUpgradingSRE\",alertname!=\"DNSErrors05MinSRE\",alertname!=\"MetricsClientSendFailingSRE\"}")
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
	for _, history := range clusterVersion.Status.History {
		if history.State == configv1.CompletedUpdate {
			gotVersion = history.Version
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

// GetClusterVersion gets the existing cluster versions from the CR clusterversion
func GetClusterVersion(c client.Client) (*configv1.ClusterVersion, error) {
	cvList := &configv1.ClusterVersionList{}
	err := c.List(context.TODO(), cvList)
	if err != nil {
		return nil, err
	}

	// ClusterVersion is a singleton
	for _, cv := range cvList.Items {
		return &cv, nil
	}

	return nil, errors.NewNotFound(schema.GroupResource{Group: configv1.GroupName, Resource: "ClusterVersion"}, "ClusterVersion")
}

// hasUpgradeCommenced checks if the upgrade has commenced
func hasUpgradeCommenced(c client.Client, uc *upgradev1alpha1.UpgradeConfig) (bool, error) {
	cv, err := GetClusterVersion(c)
	if err != nil {
		return false, err
	}

	if !isEqualVersion(cv, uc) {
		return false, nil
	}

	return true, nil
}

// nodeDrained checks if the nodes are being drained successfully during the upgrade
func nodeDrained(c client.Client, drainTimeOut time.Duration, uc *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {

	podDisruptionBudgetAtLimit := false
	pdbList := &policyv1beta1.PodDisruptionBudgetList{}
	errPDB := c.List(context.TODO(), pdbList)
	if errPDB != nil {
		return false, errPDB
	}
	for _, pdb := range pdbList.Items {
		if pdb.Status.DesiredHealthy == pdb.Status.ExpectedPods {
			podDisruptionBudgetAtLimit = true
		}
	}

	var drainStarted metav1.Time
	nodeList := &corev1.NodeList{}
	errNode := c.List(context.TODO(), nodeList)
	if errNode != nil {
		return false, errNode
	}
	for _, node := range nodeList.Items {
		if node.Spec.Unschedulable && len(node.Spec.Taints) > 0 {
			for _, n := range node.Spec.Taints {
				if n.Effect == corev1.TaintEffectNoSchedule {
					drainStarted = *n.TimeAdded
					if drainStarted.Add(time.Duration(drainTimeOut)*time.Minute).Before(metav1.Now().Time) && !podDisruptionBudgetAtLimit {
						logger.Info(fmt.Sprintf("The node cannot be drained within %d minutes.", int64(drainTimeOut)))
						return false, nil
					}
				}
			}
		}
	}
	return true, nil
}

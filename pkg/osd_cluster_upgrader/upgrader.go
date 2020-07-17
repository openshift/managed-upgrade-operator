package osd_cluster_upgrader

import (
	"context"
	"fmt"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/blang/semver"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cluster-version-operator/pkg/cincinnati"
	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	operatorv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/maintenance"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/scaler"
)

const (
	TIMEOUT_SCALE_EXTRAL_NODES = 30 * time.Minute
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
type UpgradeStep func(client.Client, scaler.Scaler, metrics.Metrics, maintenance.Maintenance, *upgradev1alpha1.UpgradeConfig, logr.Logger) (bool, error)

// Represents the order in which to undertake upgrade steps
type UpgradeStepOrdering []upgradev1alpha1.UpgradeConditionType

func NewClient(c client.Client, mc metrics.Metrics) (*osdClusterUpgrader, error) {
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
}

// ClusterHealthCheck performs cluster healthy check
func PreClusterHealthCheck(c client.Client, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
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

// This will scale up new workers to ensure customer capacity while upgrading.
func EnsureExtraUpgradeWorkers(c client.Client, s scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := hasUpgradeCommenced(c, upgradeConfig)
	if err != nil {
		return false, err
	}
	desired := upgradeConfig.Spec.Desired
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("ClusterVersion is already set to Channel %s Version %s, skipping %s", desired.Channel, desired.Version, upgradev1alpha1.UpgradeScaleUpExtraNodes))
		return true, nil
	}

	isScaled, err := s.EnsureScaleUpNodes(c, TIMEOUT_SCALE_EXTRAL_NODES, logger)
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
func CommenceUpgrade(c client.Client, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
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

// Create the maintenance window for control plane
func CreateControlPlaneMaintWindow(c client.Client, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	endTime := time.Now().Add(90 * time.Minute)
	err := m.StartControlPlane(endTime, upgradeConfig.Spec.Desired.Version)
	if err != nil {
		return false, err
	}

	return true, nil
}

// Remove the maintenance window for control plane
func RemoveControlPlaneMaintWindow(c client.Client, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	err := m.End()
	if err != nil {
		return false, err
	}

	return true, nil
}

// Create the maintenance window for workers
func CreateWorkerMaintWindow(c client.Client, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
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

	maintenanceDurationPerNode := 8 * time.Minute
	workerMaintenanceExpectedDuration := time.Duration(pendingWorkerCount) * maintenanceDurationPerNode
	endTime := time.Now().Add(workerMaintenanceExpectedDuration)
	err = m.StartWorker(endTime, upgradeConfig.Spec.Desired.Version)
	if err != nil {
		return false, err
	}

	return true, nil
}

// This check whether all the worker nodes are ready with new config
func AllWorkersUpgraded(c client.Client, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	ok, err := nodesUpgraded(c, "worker", logger)
	if err != nil {
		return false, err
	}

	silenceActive, err := m.IsActive()
	if err != nil {
		return false, err
	}

	if !ok {
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
	return true, nil
}

// This will scale down the extra workers added pre upgrade.
func RemoveExtraScaledNodes(c client.Client, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	isScaled, err := scaler.EnsureScaleDownNodes(c, logger)
	if err != nil {
		logger.Error(err, "failed to get upgrade extra machinesets")
		return false, err
	}

	return isScaled, nil
}

// This will update the 3rd subscriptions
func UpdateSubscriptions(c client.Client, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
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
func PostUpgradeVerification(c client.Client, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	ok, err := performUpgradeVerification(c, logger)
	if err != nil || !ok {
		metricsClient.UpdateMetricClusterVerificationFailed(upgradeConfig.Name)
		return false, err
	}

	metricsClient.UpdateMetricClusterVerificationSucceeded(upgradeConfig.Name)
	return true, nil
}

// performPostUpgradeVerification verify all replicasets are at expected counts and all daemonsets are at expected counts
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

// Remove maintenance
func RemoveMaintWindow(c client.Client, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	err := m.End()
	if err != nil {
		return false, err
	}

	return true, nil
}

// This perform cluster health check after upgrade
func PostClusterHealthCheck(c client.Client, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	ok, err := performClusterHealthCheck(c, metricsClient, logger)
	if err != nil || !ok {
		metricsClient.UpdateMetricClusterCheckFailed(upgradeConfig.Name)
		return false, err
	}

	metricsClient.UpdateMetricClusterCheckSucceeded(upgradeConfig.Name)
	return true, nil
}

// Check whether nodes are upgraded or not
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

// This check whether control plane is upgraded or not. The ClusterVersion reports when cvo and master nodes are upgraded.
func ControlPlaneUpgraded(c client.Client, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
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

	upgradeTimeout := 90 * time.Minute
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
		return true, nil
	}

	return false, nil
}

// This trigger the upgrade process
func (cu osdClusterUpgrader) UpgradeCluster(upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (upgradev1alpha1.UpgradePhase, *upgradev1alpha1.UpgradeCondition, error) {
	logger.Info("Upgrading cluster")

	for _, key := range cu.Ordering {

		logger.Info(fmt.Sprintf("Performing %s", key))

		result, err := cu.Steps[key](cu.client, cu.scaler, cu.metrics, cu.maintenance, upgradeConfig, logger)

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

// ValidateUpgradeConfig will run the validation steps which defined in performValidateUpgradeConfig
func ValidateUpgradeConfig(c client.Client, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := hasUpgradeCommenced(c, upgradeConfig)
	if err != nil {
		return false, err
	}
	desired := upgradeConfig.Spec.Desired
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("ClusterVersion is already set to Channel %s Version %s, skipping %s", desired.Channel, desired.Version, upgradev1alpha1.UpgradeValidated))
		return true, nil
	}

	ok, err := performValidateUpgradeConfig(c, upgradeConfig, logger)
	if err != nil || !ok {
		metricsClient.UpdateMetricValidationFailed(upgradeConfig.Name)
		return false, err
	}

	metricsClient.UpdateMetricValidationSucceeded(upgradeConfig.Name)
	return true, nil
}

// TODO move to https://github.com/openshift/managed-cluster-validating-webhooks
// performValidateUpgradeConfig will validate the UpgradeConfig, the desired version should be grater than or equal to the current version
func performValidateUpgradeConfig(c client.Client, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (result bool, err error) {

	logger.Info("Validating upgradeconfig")
	clusterVersion := &configv1.ClusterVersion{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "version"}, clusterVersion)
	if err != nil {
		logger.Info("failed to get clusterversion")
		logger.Error(err, "failed to get clusterversion")
		return false, err
	}

	//TODO get available version from ocm api like : ocm get "https://api.openshift.com/api/clusters_mgmt/v1/versions" --parameter search="enabled='t'"

	//Get current version, then compare
	current, err := GetCurrentVersion(clusterVersion)
	if err != nil {
		return false, err
	}
	logger.Info(fmt.Sprintf("current version is %s", current))
	if len(current) == 0 {
		return false, fmt.Errorf("failed to get current version")
	}
	// If the version match, it means it's already upgraded or at least control plane is upgraded.
	if current == upgradeConfig.Spec.Desired.Version {
		logger.Info("the expected version match current version")
		return false, fmt.Errorf("cluster is already on version %s", current)
	}

	// Compare the versions, if the current version is greater than desired, failed the validation, we don't support version rollback
	versions := []string{current, upgradeConfig.Spec.Desired.Version}
	logger.Info("compare two versions")
	sort.Strings(versions)
	if versions[0] != current {
		logger.Info(fmt.Sprintf("validation failed, desired version %s is older than current version %s", upgradeConfig.Spec.Desired.Version, current))
		return false, fmt.Errorf("desired version %s is older than current version %s", upgradeConfig.Spec.Desired.Version, current)
	}

	// Find the available versions from cincinnati
	clusterId, err := uuid.Parse(string(clusterVersion.Spec.ClusterID))
	if err != nil {
		return false, err
	}
	upstreamURI, err := url.Parse(string(clusterVersion.Spec.Upstream))
	if err != nil {
		return false, err
	}
	currentVersion, err := semver.Parse(current)
	if err != nil {
		return false, err
	}

	updates, err := cincinnati.NewClient(clusterId, nil, nil).GetUpdates(upstreamURI, runtime.GOARCH, upgradeConfig.Spec.Desired.Channel, currentVersion)
	if err != nil {
		return false, err
	}

	var cvoUpdates []configv1.Update
	for _, update := range updates {
		cvoUpdates = append(cvoUpdates, configv1.Update{
			Version: update.Version.String(),
			Image:   update.Image,
		})
	}
	// Check whether the desired version exists in availableUpdates

	found := false
	for _, v := range cvoUpdates {
		if v.Version == upgradeConfig.Spec.Desired.Version && !v.Force {
			found = true
		}
	}

	if !found {
		logger.Info(fmt.Sprintf("failed to find the desired version %s in channel %s", upgradeConfig.Spec.Desired.Version, upgradeConfig.Spec.Desired.Channel))
		//We need update the condition
		errMsg := fmt.Sprintf("cannot find version %s in available updates", upgradeConfig.Spec.Desired.Version)
		return false, fmt.Errorf(errMsg)
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

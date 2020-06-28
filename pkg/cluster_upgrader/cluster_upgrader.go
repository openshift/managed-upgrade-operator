package cluster_upgrader

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openshift/managed-upgrade-operator/pkg/maintenance"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"

	"github.com/blang/semver"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/cluster-version-operator/pkg/cincinnati"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	operatorv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	once                sync.Once
	osdUpgradeSteps     UpgradeSteps
	UpgradeStepOrdering = []upgradev1alpha1.UpgradeConditionType{
		upgradev1alpha1.UpgradeValidated,
		upgradev1alpha1.UpgradePreHealthCheck,
		upgradev1alpha1.UpgradeScaleUpExtraNodes,
		upgradev1alpha1.CommenceUpgrade,
		upgradev1alpha1.ControlPlaneMaintWindow,
		upgradev1alpha1.ControlPlaneUpgraded,
		upgradev1alpha1.AllMasterNodesUpgraded,
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

const (
	TIMEOUT_SCALE_EXTRAL_NODES = 30 * time.Minute
	LABEL_UPGRADE              = "upgrade.managed.openshift.io"
)

// Interface describing the functions of a cluster upgrader.
//go:generate mockgen -destination=mocks/cluster_upgrader.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/cluster_upgrader ClusterUpgrader
type ClusterUpgrader interface {
	UpgradeCluster(upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) error
}

//go:generate mockgen -destination=mocks/cluster_upgrader_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/cluster_upgrader ClusterUpgraderBuilder
type ClusterUpgraderBuilder interface {
	NewClient(client client.Client) (ClusterUpgrader, error)
}

func NewBuilder() ClusterUpgraderBuilder {
	return &clusterUpgraderBuilder{
		maintenanceBuilder: maintenance.NewBuilder(),
	}
}

// Represents a named series of steps as part of an upgrade process
type UpgradeSteps map[upgradev1alpha1.UpgradeConditionType]UpgradeStep

// Represents an individual step in the upgrade process
type UpgradeStep func(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error)

type clusterUpgraderBuilder struct {
	maintenanceBuilder maintenance.MaintenanceBuilder
}

func (cub *clusterUpgraderBuilder) NewClient(c client.Client) (ClusterUpgrader, error) {
	m, err := cub.maintenanceBuilder.NewClient(c)
	ms := &metrics.Counter{}
	if err != nil {
		return nil, err
	}

	once.Do(func() {
		osdUpgradeSteps = map[upgradev1alpha1.UpgradeConditionType]UpgradeStep{
			upgradev1alpha1.UpgradeValidated:              ValidateUpgradeConfig,
			upgradev1alpha1.UpgradePreHealthCheck:         PreClusterHealthCheck,
			upgradev1alpha1.UpgradeScaleUpExtraNodes:      EnsureExtraUpgradeWorkers,
			upgradev1alpha1.ControlPlaneMaintWindow:       CreateControlPlaneMaintWindow,
			upgradev1alpha1.CommenceUpgrade:               CommenceUpgrade,
			upgradev1alpha1.ControlPlaneUpgraded:          ControlPlaneUpgraded,
			upgradev1alpha1.AllMasterNodesUpgraded:        AllMastersUpgraded,
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
	return &clusterUpgrader{
		Steps:       osdUpgradeSteps,
		client:      c,
		maintenance: m,
		metrics:     ms,
	}, nil
}

// An cluster upgrader implementing the ClusterUpgrader interface
type clusterUpgrader struct {
	Steps       UpgradeSteps
	client      client.Client
	maintenance maintenance.Maintenance
	metrics     metrics.Metrics
}

// Ordering returns the ordering of predicates.
func Ordering() []upgradev1alpha1.UpgradeConditionType {
	return UpgradeStepOrdering
}

// ClusterHealthCheck performs cluster healthy check
func PreClusterHealthCheck(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	ok, err := performClusterHealthCheck(c, logger)
	if err != nil || !ok {
		metricsClient.UpdateMetricClusterCheckFailed(upgradeConfig.Name)
		return false, err
	}

	metricsClient.UpdateMetricClusterCheckSucceeded(upgradeConfig.Name)
	return true, nil
}

// This will create a new machineset with 1 extra replicas for workers in every region
func EnsureExtraUpgradeWorkers(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	upgradeMachinesets := &machineapi.MachineSetList{}

	err := c.List(context.TODO(), upgradeMachinesets, []client.ListOption{
		client.InNamespace("openshift-machine-api"),
		client.MatchingLabels{LABEL_UPGRADE: "true"},
	}...)
	if err != nil {
		logger.Error(err, "failed to get upgrade extra machinesets")
		return false, err
	}
	originalMachineSets := &machineapi.MachineSetList{}

	err = c.List(context.TODO(), originalMachineSets, []client.ListOption{
		client.InNamespace("openshift-machine-api"),
		client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
	}...)
	if err != nil {
		logger.Error(err, "failed to get original machinesets")
		return false, err
	}
	if len(originalMachineSets.Items) == 0 {
		logger.Info("failed to get machineset")
		return false, fmt.Errorf("failed to get original machineset")
	}

	updated := false
	for _, ms := range originalMachineSets.Items {

		found := false
		for _, ums := range upgradeMachinesets.Items {
			if ums.Name == ms.Name+"-upgrade" {
				found = true
			}
		}
		if found {
			logger.Info(fmt.Sprintf("machineset for upgrade already created :%s", ms.Name))
			continue
		}
		updated = true
		replica := int32(1)
		newMs := ms.DeepCopy()

		newMs.ObjectMeta = metav1.ObjectMeta{
			Name:      ms.Name + "-upgrade",
			Namespace: ms.Namespace,
			Labels: map[string]string{
				LABEL_UPGRADE: "true",
			},
		}
		newMs.Spec.Replicas = &replica
		newMs.Spec.Template.Labels[LABEL_UPGRADE] = "true"
		newMs.Spec.Selector.MatchLabels[LABEL_UPGRADE] = "true"
		logger.Info(fmt.Sprintf("creating machineset %s for upgrade", newMs.Name))

		err = c.Create(context.TODO(), newMs)
		if err != nil {
			logger.Error(err, "failed to create machineset")
			return false, err
		}

	}
	if updated {
		// New machineset created, machines must not ready at the moment, so skip following steps
		return false, nil
	}
	nodes := &corev1.NodeList{}
	err = c.List(context.TODO(), nodes)
	if err != nil {
		logger.Error(err, "failed to list nodes")
		return false, err
	}
	allNodeReady := true
	for _, ms := range upgradeMachinesets.Items {
		//We assume the create time is the start time for scale up extra compute nodes
		startTime := ms.CreationTimestamp
		if ms.Status.Replicas != ms.Status.ReadyReplicas {

			if time.Now().After(startTime.Time.Add(TIMEOUT_SCALE_EXTRAL_NODES)) {
				//TODO send out timeout alerts
				logger.Info("machineset provisioning timout")
			}
			logger.Info(fmt.Sprintf("not all machines are ready for machineset:%s", ms.Name))
			return false, nil
		}
		machines := &machineapi.MachineList{}
		err := c.List(context.TODO(), machines, []client.ListOption{
			client.InNamespace("openshift-machine-api"),
			client.MatchingLabels{LABEL_UPGRADE: "true"},
		}...)
		if err != nil {
			logger.Error(err, "failed to list extra upgrade machine")
			return false, err
		}
		nodeReady := false
		var nodeName string
		for _, node := range nodes.Items {
			if node.Annotations["machine.openshift.io/machine"] == "openshift-machine-api/"+machines.Items[0].Name {
				for _, con := range node.Status.Conditions {
					if con.Type == corev1.NodeReady && con.Status == corev1.ConditionTrue {
						nodeReady = true
						nodeName = node.Name
					}
				}

			}

		}
		if !nodeReady {
			allNodeReady = false
			if time.Now().After(startTime.Time.Add(TIMEOUT_SCALE_EXTRAL_NODES)) {
				logger.Info("node is not ready within 30mins")
				//TODO send out timeout alerts
				return false, fmt.Errorf("timeout waiting for node:%s to become ready", nodeName)

			}
		}

	}
	if !allNodeReady {
		return false, nil
	}

	return allNodeReady, nil

}

// CommenceUpgrade will update the clusterversion object to apply the desired version to trigger real OCP upgrade
func CommenceUpgrade(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	clusterVersion := &configv1.ClusterVersion{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: "version"}, clusterVersion)
	if err != nil {
		return false, err
	}
	if clusterVersion.Spec.DesiredUpdate != nil &&
		clusterVersion.Spec.DesiredUpdate.Version == upgradeConfig.Spec.Desired.Version &&
		clusterVersion.Spec.Channel == upgradeConfig.Spec.Desired.Channel {
		return true, nil
	}
	// https://issues.redhat.com/browse/OSD-3442
	clusterVersion.Spec.Overrides = []configv1.ComponentOverride{}
	clusterVersion.Spec.DesiredUpdate = &configv1.Update{Version: upgradeConfig.Spec.Desired.Version}
	clusterVersion.Spec.Channel = upgradeConfig.Spec.Desired.Channel

	//Record the timestamp when we start the upgrade
	metricsClient.UpdateMetricUpgradeStartTime(time.Now(), upgradeConfig.Name)
	err = c.Update(context.TODO(), clusterVersion)
	if err != nil {
		return false, err
	}
	return true, nil
}

// Create the maintenance window for control plane
func CreateControlPlaneMaintWindow(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	endTime := time.Now().Add(90 * time.Minute)
	err := m.StartControlPlane(endTime)
	if err != nil {
		return false, err
	}

	return true, nil
}

// Remove the maintenance window for control plane
func RemoveControlPlaneMaintWindow(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	err := m.End()
	if err != nil {
		return false, err
	}

	return true, nil
}

// Create the maintenance window for workers
func CreateWorkerMaintWindow(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	configPool := &machineconfigapi.MachineConfigPool{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: "worker"}, configPool)
	if err != nil {
		return false, nil
	}

	pendingWorkerCount := configPool.Status.MachineCount - configPool.Status.UpdatedMachineCount
	maintenanceDurationPerNode := 8 * time.Minute
	workerMaintenanceExpectedDuration := time.Duration(pendingWorkerCount) * maintenanceDurationPerNode
	endTime := time.Now().Add(workerMaintenanceExpectedDuration)
	err = m.StartWorker(endTime)
	if err != nil {
		return false, err
	}

	return true, nil
}

// This check whether all the master nodes are ready with new config
func AllMastersUpgraded(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {

	return nodesUpgraded(c, "master", logger)

}

// This check whether all the worker nodes are ready with new config
func AllWorkersUpgraded(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	ok, err := nodesUpgraded(c, "worker", logger)
	if err != nil || !ok {
		return false, err
	}

	metricsClient.UpdateMetricNodeUpgradeEndTime(time.Now(), upgradeConfig.Name)
	return true, nil
}

// This will remove the extra worker nodes we added before kick off upgrade
func RemoveExtraScaledNodes(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	upgradeMachinesets := &machineapi.MachineSetList{}

	err := c.List(context.TODO(), upgradeMachinesets, []client.ListOption{
		client.InNamespace("openshift-machine-api"),
		client.MatchingLabels{LABEL_UPGRADE: "true"},
	}...)
	if err != nil {
		logger.Error(err, "failed to get upgrade extra machinesets")
		return false, err
	}
	for _, item := range upgradeMachinesets.Items {
		err = c.Delete(context.TODO(), &item)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

// This will update the 3rd subscriptions
func UpdateSubscriptions(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
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
func PostUpgradeVerification(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
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
	if len(dsList.Items) != readyDS {
		logger.Info(fmt.Sprintf("not all daemonset are ready:expected number :%v , ready number %v", len(dsList.Items), readyDS))
		return false, nil
	}

	return true, nil
}

// Remove maintenance
func RemoveMaintWindow(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	err := m.End()
	if err != nil {
		return false, err
	}

	return true, nil
}

// This perform cluster health check after upgrade
func PostClusterHealthCheck(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	ok, err := performClusterHealthCheck(c, logger)
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

// This check whether control plane is upgraded or not
func ControlPlaneUpgraded(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	clusterVersion := &configv1.ClusterVersion{}

	err := c.Get(context.TODO(), types.NamespacedName{Name: "version"}, clusterVersion)
	if err != nil {
		return false, err
	}
	for _, c := range clusterVersion.Status.History {
		if c.State == configv1.CompletedUpdate && c.Version == upgradeConfig.Spec.Desired.Version {
			// send controlplane upgrade complete timestamp
			metricsClient.UpdateMetricControlPlaneEndTime(time.Now(), upgradeConfig.Name)
			return true, nil
		}
	}
	return false, nil

}

// This trigger the upgrade process
func (cu clusterUpgrader) UpgradeCluster(upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) error {

	logger.Info("upgrading cluster")
	history := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
	conditions := history.Conditions

	if history.Phase != upgradev1alpha1.UpgradePhaseUpgrading {
		history.Phase = upgradev1alpha1.UpgradePhaseUpgrading
		history.StartTime = &metav1.Time{Time: time.Now()}
		upgradeConfig.Status.History.SetHistory(*history)
		err := cu.client.Status().Update(context.TODO(), upgradeConfig)
		if err != nil {
			logger.Error(err, "failed to update upgradeconfig")
		}
	}

	for _, key := range Ordering() {

		logger.Info(fmt.Sprintf("Perform %s", key))

		condition := conditions.GetCondition(key)
		if condition == nil {
			logger.Info(fmt.Sprintf("Adding %s condition", key))
			condition = newUpgradeCondition(fmt.Sprintf("start %s", key), fmt.Sprintf("start %s", key), key, corev1.ConditionFalse)
			condition.StartTime = &metav1.Time{Time: time.Now()}
			conditions.SetCondition(*condition)
			history.Conditions = conditions
			upgradeConfig.Status.History.SetHistory(*history)
			err := cu.client.Status().Update(context.TODO(), upgradeConfig)
			if err != nil {
				return err
			}
		}
		if condition.Status == corev1.ConditionTrue {
			logger.Info(fmt.Sprintf("%s already done, skip", key))
			continue
		}
		result, err := cu.Steps[key](cu.client, cu.metrics, cu.maintenance, upgradeConfig, logger)

		if err != nil {
			logger.Error(err, fmt.Sprintf("error when %s", key))
			condition.Reason = fmt.Sprintf("%s not done", key)
			condition.Message = err.Error()
			conditions.SetCondition(*condition)
			history.Conditions = conditions
			upgradeConfig.Status.History.SetHistory(*history)
			err = cu.client.Status().Update(context.TODO(), upgradeConfig)
			if err != nil {
				return err
			}
			return err
		}
		if result {
			condition.CompleteTime = &metav1.Time{Time: time.Now()}
			condition.Reason = fmt.Sprintf("%s succeed", key)
			condition.Message = fmt.Sprintf("%s succeed", key)
			condition.Status = corev1.ConditionTrue
			conditions.SetCondition(*condition)
			history.Conditions = conditions
			upgradeConfig.Status.History.SetHistory(*history)
			err = cu.client.Status().Update(context.TODO(), upgradeConfig)
			if err != nil {
				return err
			}
		} else {
			logger.Info(fmt.Sprintf("%s not done, skip following steps", key))
			condition.Reason = fmt.Sprintf("%s not done", key)
			condition.Message = fmt.Sprintf("%s still in progress", key)
			conditions.SetCondition(*condition)
			history.Conditions = conditions
			upgradeConfig.Status.History.SetHistory(*history)
			err = cu.client.Status().Update(context.TODO(), upgradeConfig)
			if err != nil {
				return err
			}
			return nil
		}
	}
	history.Phase = upgradev1alpha1.UpgradePhaseUpgraded
	history.CompleteTime = &metav1.Time{Time: time.Now()}
	upgradeConfig.Status.History.SetHistory(*history)
	err := cu.client.Status().Update(context.TODO(), upgradeConfig)
	if err != nil {
		return err
	}
	return nil

}

// check several things about the cluster and report problems
// * critical alerts
// * degraded operators (if there are critical alerts only)
func performClusterHealthCheck(c client.Client, logger logr.Logger) (bool, error) {
	sa := &corev1.ServiceAccount{}

	err := c.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-monitoring", Name: "prometheus-k8s"}, sa)
	if err != nil {
		return false, fmt.Errorf("Unable to fetch prometheus-k8s service account: %s", err)
	}

	tokenSecret := ""
	for _, secret := range sa.Secrets {
		if strings.HasPrefix(secret.Name, "prometheus-k8s-token") {
			tokenSecret = secret.Name
		}
	}
	if len(tokenSecret) == 0 {
		return false, fmt.Errorf("failed to find token secret for prommetheus-k8s SA")
	}

	logger.Info(fmt.Sprintf("found out secret %s", tokenSecret))

	secret := &corev1.Secret{}

	err = c.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-monitoring", Name: tokenSecret}, secret)
	if err != nil {
		return false, fmt.Errorf("Unable to fetch secret %s: %s", tokenSecret, err)
	}

	token := secret.Data[corev1.ServiceAccountTokenKey]

	route := &routev1.Route{}
	err = c.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-monitoring", Name: "prometheus-k8s"}, route)
	if err != nil {
		return false, err
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	hclient := http.Client{Transport: tr}
	promurl := "https://" + route.Spec.Host + "/api/v1/query"

	req, err := http.NewRequest("GET", promurl, nil)
	if err != nil {
		return false, fmt.Errorf("Could not query Prometheus: %s", err)
	}
	q := req.URL.Query()
	alertQuery := "ALERTS{alertstate=\"firing\",severity=\"critical\",namespace=~\"^openshift.*|^kube.*|^default$\",namespace!=\"openshift-customer-monitoring\",alertname!=\"ClusterUpgradingSRE\",alertname!=\"DNSErrors05MinSRE\",alertname!=\"MetricsClientSendFailingSRE\"}"
	q.Add("query", alertQuery)

	req.URL.RawQuery = q.Encode()
	req.Header.Add("Authorization", "Bearer "+string(token))
	resp, err := hclient.Do(req)
	if err != nil {
		return false, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("Error when querying Prometheus alerts: %s", err)
	}

	logger.Info(fmt.Sprintf("alerts : %s", body))
	alerts := &AlertResponse{}

	err = json.Unmarshal(body, alerts)
	if err != nil {
		return false, err
	}

	if len(alerts.Data.Result) > 0 {
		logger.Info("there are critical alerts exists, cannot upgrade now")
		// Send the metrics for the cluster check failed if we have opening alerts
		return false, fmt.Errorf("there are %d critical alerts", len(alerts.Data.Result))
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

type AlertResponse struct {
	Status string    `json:"status"`
	Data   AlertData `json:"data"`
}

type AlertData struct {
	Result []interface{} `json:"result"`
}

// ValidateUpgradeConfig will run the validation steps which defined in performValidateUpgradeConfig
func ValidateUpgradeConfig(c client.Client, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error){
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

	logger.Info("validating upgradeconfig")
	clusterVersion := &configv1.ClusterVersion{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "version"}, clusterVersion)
	if err != nil {
		logger.Info("failed to get clusterversion")
		logger.Error(err, "failed to get clusterversion")
		return false, err
	}

	//TODO get available version from ocm api like : ocm get "https://api.openshift.com/api/clusters_mgmt/v1/versions" --parameter search="enabled='t'"

	//Get current version, then compare
	current := getCurrentVersion(clusterVersion)
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
		logger.Info(fmt.Sprintf("validation failed, current version %s is greater than desired %s", current, upgradeConfig.Spec.Desired.Version))
		return false, fmt.Errorf("desired version %s is greater than current version %s", upgradeConfig.Spec.Desired.Version, current)
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

func getCurrentVersion(clusterVersion *configv1.ClusterVersion) string {
	for _, history := range clusterVersion.Status.History {
		if history.State == configv1.CompletedUpdate {
			return history.Version
		}
	}
	return ""
}

// This return the current upgrade status
func IsClusterUpgrading(c client.Client, version string) (bool, error) {
	clusterVersion := &configv1.ClusterVersion{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: "version"}, clusterVersion)
	if err != nil {
		return false, err
	}
	for _, c := range clusterVersion.Status.Conditions {
		if c.Type == configv1.OperatorProgressing && c.Status == configv1.ConditionTrue && version != clusterVersion.Spec.DesiredUpdate.Version {
			return true, nil

		}
	}
	return false, nil
}

func newUpgradeCondition(reason, msg string, conditionType upgradev1alpha1.UpgradeConditionType, s corev1.ConditionStatus) *upgradev1alpha1.UpgradeCondition {
	return &upgradev1alpha1.UpgradeCondition{
		Type:    conditionType,
		Status:  s,
		Reason:  reason,
		Message: msg,
	}
}

// TODO readyToUpgrade checks whether it's ready to upgrade based on the scheduling
func IsReadyToUpgrade(upgradeConfig *upgradev1alpha1.UpgradeConfig) bool {
	return true
}

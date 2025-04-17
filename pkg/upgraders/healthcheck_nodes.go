package upgraders

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ManuallyCordonedNodes(metricsClient metrics.Metrics, machinery machinery.Machinery, c client.Client, ug *upgradev1alpha1.UpgradeConfig, logger logr.Logger, version string) ([]string, error) {
	nodes := &corev1.NodeList{}
	cops := &client.ListOptions{
		Raw: &metav1.ListOptions{
			LabelSelector: "node-role.kubernetes.io/worker, !node-role.kubernetes.io/infra",
		},
	}

	// Get current upgrade state
	history := ug.Status.History.GetHistory(ug.Spec.Desired.Version)
	state := string(history.Phase)

	// Get the list of worker nodes
	err := c.List(context.TODO(), nodes, cops)
	if err != nil {
		logger.Info("Unable to fetch node list")
		// Use PrecedingVersion as the versionLabel here because preflight check is performed before the upgrade
		metricsClient.UpdateMetricHealthcheckFailed(ug.Name, metrics.ClusterNodeQueryFailed, version, state)
		return nil, err
	}

	// Check if worker node has been manually cordoned.
	var manuallyCordonNodes []string
	isHealthCheckFailed := false
	for _, node := range nodes.Items {
		node := node
		cordonResult := machinery.IsNodeCordoned(&node)
		if cordonResult.IsCordoned && !machinery.IsNodeUpgrading(&node) {
			//Node has been manually cordoned, set the flag and record the node name
			isHealthCheckFailed = true
			manuallyCordonNodes = append(manuallyCordonNodes, node.Name)
		}
	}

	if isHealthCheckFailed {
		// Manually cordon node check failed, fail the healthcheck and return failed nodes
		// Use PrecedingVersion as the versionLabel here because preflight check is performed before the upgrade
		metricsClient.UpdateMetricHealthcheckFailed(ug.Name, metrics.ClusterNodesManuallyCordoned, version, state)
		return manuallyCordonNodes, fmt.Errorf("cordoned nodes: %s", strings.Join(manuallyCordonNodes, ", "))
	}
	logger.Info("Prehealth check for manually cordoned node passed")
	metricsClient.UpdateMetricHealthcheckSucceeded(ug.Name, metrics.ClusterNodeQueryFailed, version, state)
	metricsClient.UpdateMetricHealthcheckSucceeded(ug.Name, metrics.ClusterNodesManuallyCordoned, version, state)
	return nil, nil
}

func NodeUnschedulableTaints(metricsClient metrics.Metrics, machinery machinery.Machinery, c client.Client, ug *upgradev1alpha1.UpgradeConfig, logger logr.Logger, version string) ([]string, error) {
	nodes := &corev1.NodeList{}
	cops := &client.ListOptions{}

	// Get current upgrade state
	history := ug.Status.History.GetHistory(ug.Spec.Desired.Version)
	state := string(history.Phase)

	// Get the list of worker nodes
	err := c.List(context.TODO(), nodes, cops)
	if err != nil {
		logger.Info("Unable to fetch node list")
		// Use PrecedingVersion as the versionLabel here because preflight check is performed before the upgrade
		metricsClient.UpdateMetricHealthcheckFailed(ug.Name, metrics.ClusterNodeQueryFailed, version, state)
		return nil, err
	}

	// Check if worker node has been manually cordoned.
	var memPressureNodes []string
	var diskPressureNodes []string
	var pidPressureNodes []string
	var unschedulableNodes []string
	isHealthCheckFailed := false
	for _, node := range nodes.Items {
		node := node
		if machinery.HasMemoryPressure(&node) {
			//Node has memory pressure and unschedulable taint , set the flag and record the node name
			isHealthCheckFailed = true
			memPressureNodes = append(memPressureNodes, node.Name)
		}

		if machinery.HasDiskPressure(&node) {
			//Node has disk pressure and unschedulable taint, set the flag and record the node name
			isHealthCheckFailed = true
			diskPressureNodes = append(diskPressureNodes, node.Name)
		}

		if machinery.HasPidPressure(&node) {
			//Node has PID pressure and unschedulable taint, set the flag and record the node name
			isHealthCheckFailed = true
			pidPressureNodes = append(pidPressureNodes, node.Name)
		}
	}

	if isHealthCheckFailed {
		// Node unschedulable taints check failed, fail the healthcheck and return failed nodes
		logger.Info(" Unschedulable node taints check failed")
		// Use PrecedingVersion as the versionLabel here because preflight check is performed before the upgrade
		metricsClient.UpdateMetricHealthcheckFailed(ug.Name, metrics.ClusterNodesTaintedUnschedulable, version, state)
		if len(memPressureNodes) > 0 {
			logger.Info(fmt.Sprintf("%s has/have memory pressure", strings.Join(memPressureNodes, ", ")))
			unschedulableNodes = append(unschedulableNodes, memPressureNodes...)
		}

		if len(diskPressureNodes) > 0 {
			logger.Info(fmt.Sprintf("%s has/have disk pressure", strings.Join(diskPressureNodes, ", ")))
			unschedulableNodes = append(unschedulableNodes, diskPressureNodes...)
		}

		if len(pidPressureNodes) > 0 {
			logger.Info(fmt.Sprintf("%s has/have PID pressure", strings.Join(pidPressureNodes, ", ")))
			unschedulableNodes = append(unschedulableNodes, pidPressureNodes...)
		}

		return unschedulableNodes, fmt.Errorf("unschedulable taints on nodes: %s", strings.Join(unschedulableNodes, ", "))
	}
	logger.Info("Prehealth check for unschedulable node taints passed")
	metricsClient.UpdateMetricHealthcheckSucceeded(ug.Name, metrics.ClusterNodeQueryFailed, version, state)
	metricsClient.UpdateMetricHealthcheckSucceeded(ug.Name, metrics.ClusterNodesTaintedUnschedulable, version, state)
	return nil, nil
}

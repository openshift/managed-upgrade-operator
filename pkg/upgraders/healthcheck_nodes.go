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

func ManuallyCordonedNodes(metricsClient metrics.Metrics, machinery machinery.Machinery, c client.Client, ug *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	nodes := &corev1.NodeList{}
	cops := &client.ListOptions{
		Raw: &metav1.ListOptions{
			LabelSelector: "node-role.kubernetes.io/worker, !node-role.kubernetes.io/infra",
		},
	}
	// Get the list of worker nodes
	err := c.List(context.TODO(), nodes, cops)
	if err != nil {
		logger.Info("Unable to fetch node list")
		metricsClient.UpdateMetricHealthcheckFailed(ug.Name, metrics.ClusternNodesManuallyCordoned)
		return false, err
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
		metricsClient.UpdateMetricHealthcheckFailed(ug.Name, metrics.ClusternNodesManuallyCordoned)
		return false, fmt.Errorf("cordoned nodes: %s", strings.Join(manuallyCordonNodes, ", "))
	}
	logger.Info("Prehealth check for manually cordoned node passed")
	return true, nil
}

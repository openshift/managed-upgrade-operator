package nodekeeper

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"os"

	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
)

var log = logf.Log.WithName("controller_nodekeeper")

// Add creates a new NodeKeeper Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNodeKeeper{
		client:               mgr.GetClient(),
		configManagerBuilder: configmanager.NewBuilder(),
		machinery:            machinery.NewMachinery(),
		metricsClientBuilder: metrics.NewBuilder(),
		scheme:               mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("nodekeeper-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Node, status change will not trigger a reconcile
	err = c.Watch(
		&source.Kind{Type: &corev1.Node{}},
		&handler.EnqueueRequestForObject{},
		IgnoreMasterPredicate)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileNodeKeeper implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileNodeKeeper{}

// ReconcileNodeKeeper reconciles a NodeKeeper object
type ReconcileNodeKeeper struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client               client.Client
	configManagerBuilder configmanager.ConfigManagerBuilder
	machinery            machinery.Machinery
	metricsClientBuilder metrics.MetricsBuilder
	scheme               *runtime.Scheme
}

// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNodeKeeper) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	operatorNamespace, err := getOperatorNamespace()
	if err != nil {
		return reconcile.Result{}, err
	}
	uc, err := getUpgradeConfigCR(r.client, operatorNamespace)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	upgradeResult, err := r.machinery.IsUpgrading(r.client, "worker")
	if err != nil {
		return reconcile.Result{}, err
	}

	history := uc.Status.History.GetHistory(uc.Spec.Desired.Version)
	if !(history.Phase == upgradev1alpha1.UpgradePhaseUpgrading && upgradeResult.IsUpgrading) {
		return reconcile.Result{}, nil
	}

	// Fetch the Node instance
	node := &corev1.Node{}
	err = r.client.Get(context.TODO(), request.NamespacedName, node)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	cfm := r.configManagerBuilder.New(r.client, operatorNamespace)
	cfg := &nodeKeeperConfig{}
	err = cfm.Into(cfg)
	if err != nil {
		return reconcile.Result{}, err
	}

	drainStartTime := getDrainStartedAtTime(node)
	isNodeDrainTimedOut := drainStartTime != nil && drainStartTime.Add(cfg.NodeDrain.GetDuration()).Before(metav1.Now().Time)

	// Initialise metrics
	metricsClient, err := r.metricsClientBuilder.NewClient(r.client)
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger := log.WithValues("Request.Name", request.Name)
	reqLogger.Info("Reconciling NodeKeeper")
	if isNodeDrainTimedOut {
		podDisruptionBudgetAtLimit := false
		pdbList := &policyv1beta1.PodDisruptionBudgetList{}
		errPDB := r.client.List(context.TODO(), pdbList)
		if errPDB != nil {
			return reconcile.Result{}, errPDB
		}
		for _, pdb := range pdbList.Items {
			if pdb.Status.DesiredHealthy == pdb.Status.ExpectedPods {
				podDisruptionBudgetAtLimit = true
			}
		}

		// Declare these vars in this scope for PDB vs normal node drain analysis
		var pdbAlertsOnNode = true
		var pdbPods *corev1.PodList
		if pdbAlerts {
			// Check if PDB pods are on node instance
			pdbPods, err = getPDBLabelPodsFromNode(r.client, pdbLabels, instance)
			if err != nil {
				return reconcile.Result{}, err
			}
			if len(pdbPods.Items) == 0 {
				pdbAlertsOnNode = false
			}
		}

		if pdbAlertsOnNode {
			// Execute PDB flow.
			reqLogger.Info(fmt.Sprintf("Found PDB alerts matching %s", pdbLabels))
			metricsClient.UpdateMetricNodeDrainFailed(uc.Name)
			return reconcile.Result{}, nil
		}

		reqLogger.Info(fmt.Sprintf("Node drain timed out %s. Alerting.", node.Name))
		metricsClient.UpdateMetricNodeDrainFailed(uc.Name)
	} else {
		metricsClient.ResetMetricNodeDrainFailed(uc.Name)
	}

	return reconcile.Result{}, nil
}

// getOperatorNamespace retrieves the operators namespace from an environment
// variable and returns it to the caller.
func getOperatorNamespace() (string, error) {
	envVarOperatorNamespace := "OPERATOR_NAMESPACE"
	ns, found := os.LookupEnv(envVarOperatorNamespace)
	if !found {
		return "", fmt.Errorf("%s must be set", envVarOperatorNamespace)
	}
	return ns, nil
}

func getUpgradeConfigCR(c client.Client, ns string) (*upgradev1alpha1.UpgradeConfig, error) {
	uCList := &upgradev1alpha1.UpgradeConfigList{}

	err := c.List(context.TODO(), uCList, &client.ListOptions{Namespace: ns})
	if err != nil {
		return nil, err
	}

	for _, uC := range uCList.Items {
		return &uC, nil
	}

	return nil, errors.NewNotFound(schema.GroupResource{Group: upgradev1alpha1.SchemeGroupVersion.Group, Resource: "UpgradeConfig"}, "UpgradeConfig")
}

func getDrainStartedAtTime(node *corev1.Node) *metav1.Time {
	var drainStartedAtTime *metav1.Time
	if node.Spec.Unschedulable && len(node.Spec.Taints) > 0 {
		for _, n := range node.Spec.Taints {
			if n.Effect == corev1.TaintEffectNoSchedule {
				drainStartedAtTime = n.TimeAdded
			}
		}
	}

	return drainStartedAtTime
}

// For clarity, use pdbLabelsType to pass around the named map type.
type pdbLabelsType map[string]string

// GetPDBLabelPodsFromNode returns a slice of pod names as strings for given pod labels and target node Name.
func getPDBLabelPodsFromNode(c client.Client, pdbLabels pdbLabelsType, node *corev1.Node) (*corev1.PodList, error) {
	// TODO: we should be able to return target node with FieldsSelector elegantly
	//	nodeMap := make(map[string]string)
	//	nodeMap["spec.nodeName="] = node.Name
	//nodeL := fields.Set(nodeMap)
	//client.MatchingFieldsSelector{Selector: nodeL.AsSelector()},
	foundPods := &corev1.PodList{}
	podList := &corev1.PodList{}
	pdbL := labels.Set(pdbLabels)

	listOpts := []client.ListOption{
		client.MatchingLabelsSelector{Selector: pdbL.AsSelector()},
	}

	err := c.List(context.TODO(), podList, listOpts...)
	if err != nil {
		return foundPods, err
	}

	for _, pod := range podList.Items {
		if pod.Spec.NodeName == node.Name {
			foundPods.Items = append(foundPods.Items, pod)
		}
	}
	return foundPods, nil
}

// GetPDBAlerts retrieves a PodDisruptionBudgetList and checks if DesiredHealthy
// is equal to ExpectedPods which indicates.
func getPDBAlertsWithLabels(c client.Client) (bool, map[string]string, error) {
	/* use cases
	maxUnavailable = 0
	minAvailable + DesiredHealthy == replica count*/
	PDBPreventingPodDeletion := false
	pdbMatchLabels := make(map[string]string)

	pdbList := &policyv1beta1.PodDisruptionBudgetList{}
	err := c.List(context.TODO(), pdbList)
	if err != nil {
		return false, nil, err
	}

	for _, pdb := range pdbList.Items {
		// TODO: handle multiple PDB objects firing
		// PDB protect pod deletion status.
		// https://github.com/kubernetes/kubernetes/blob/master/pkg/registry/core/pod/storage/eviction.go#L288-L289
		if pdb.Status.PodDisruptionsAllowed == 0 {
			PDBPreventingPodDeletion = true
			pdbMatchLabels := pdb.Spec.Selector.MatchLabels
			return PDBPreventingPodDeletion, pdbMatchLabels, nil
		}
	}
	// Return no alerts and no errors.
	return PDBPreventingPodDeletion, pdbMatchLabels, nil
}

// For clarity, use pdbLabelsType to pass around the named map type.
type pdbLabelsType map[string]string

// GetPDBLabelPodsFromNode returns a slice of pod names as strings for given pod labels and target node Name.
func getPDBLabelPodsFromNode(c client.Client, pdbLabels pdbLabelsType, node *corev1.Node) (*corev1.PodList, error) {
	// TODO: we should be able to return target node with FieldsSelector elegantly
	//	nodeMap := make(map[string]string)
	//	nodeMap["spec.nodeName="] = node.Name
	//nodeL := fields.Set(nodeMap)
	//client.MatchingFieldsSelector{Selector: nodeL.AsSelector()},
	foundPods := &corev1.PodList{}
	podList := &corev1.PodList{}
	pdbL := labels.Set(pdbLabels)

	listOpts := []client.ListOption{
		client.MatchingLabelsSelector{Selector: pdbL.AsSelector()},
	}

	err := c.List(context.TODO(), podList, listOpts...)
	if err != nil {
		return foundPods, err
	}

	for _, pod := range podList.Items {
		if pod.Spec.NodeName == node.Name {
			foundPods.Items = append(foundPods.Items, pod)
		}
	}
	return foundPods, nil
}

package upgradeconfig

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/clock"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	log = logf.Log.WithName("controller_stucknode")
)

// blank assignment to verify that StuckNode implements reconcile.Reconciler
var _ reconcile.Reconciler = &StuckNode{}

// StuckNode reconciles the StuckNode condition.
// This condition is intentionally independent of the upgrade itself since nodes can be stuck during any drain, regardless
// of which upgrade or any upgrade in progress.
type StuckNode struct {
	dynamicClient        dynamic.Interface
	kubeClient           kubernetes.Interface
	ongoingDrains        map[string]time.Time
	drainTimeoutDuration time.Duration
	clock                clock.Clock
}

func (r *StuckNode) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling stuck node checker")
	// TODO seems much cleaner to wire up logger in context like kube

	reconcileErr := r.reconcile(ctx, reqLogger)
	if reconcileErr != nil {
		// use real generated client to set only the StuckNodeControllerDegraded condition
		return reconcile.Result{}, reconcileErr
	}

	// use real generated client to set only the StuckNodeControllerDegraded condition

	return reconcile.Result{}, reconcileErr
}

func (r *StuckNode) reconcile(ctx context.Context, logger logr.Logger) error {
	allNodes, err := r.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	drainingNodes := []corev1.Node{}
	drainingNodesByName := map[string]corev1.Node{}
	for _, node := range allNodes.Items {
		// draining nodes are unschedulable
		if !node.Spec.Unschedulable {
			continue
		}
		// the MCO uses this annotation with a suffix of drain to indicate draining.
		if !strings.HasSuffix(node.Annotations["machineconfiguration.openshift.io/desiredDrain"], "drain") {
			continue
		}

		drainingNodesByName[node.Name] = node
		drainingNodes = append(drainingNodes, node)
	}

	// add nodes we don't have a record for
	for _, drainingNode := range drainingNodes {
		if _, exists := r.ongoingDrains[drainingNode.Name]; !exists {
			r.ongoingDrains[drainingNode.Name] = r.clock.Now()
		}
	}

	// remove nodes that are no longer draining
	for nodeName := range r.ongoingDrains {
		for _, drainingNode := range drainingNodes {
			if drainingNode.Name == nodeName {
				delete(r.ongoingDrains, drainingNode.Name)
			}
		}
	}

	stuckNodeNames := []string{}
	for nodeName, drainStartObservedTime := range r.ongoingDrains {
		durationOfDrainSoFar := r.clock.Now().Sub(drainStartObservedTime)
		if durationOfDrainSoFar > r.drainTimeoutDuration {
			stuckNodeNames = append(stuckNodeNames, nodeName)
		}
	}
	sort.Strings(stuckNodeNames)

	if len(stuckNodeNames) == 0 {
		// if no nodes are stuck, then simply set the condition ok
		// use real generated client to set only the StuckNode condition
		// TODO remove debugging
		logger.Info("no stuck nodes found")
		return nil
	}

	// if we have stuck nodes, iterate through each node to see if there's an obvious PDB sticking us
	failureMessages := []string{
		// by semi-coincidence, this sorts first.  We could adjust code if we wanted.
		fmt.Sprintf("identified %d stuck nodes", len(stuckNodeNames)),
	}
	for _, stuckNodeName := range stuckNodeNames {
		stuckNode := drainingNodesByName[stuckNodeName]
		interestingPods, err := interestingPodsOnStuckNode(ctx, r.kubeClient, stuckNode, r.drainTimeoutDuration)
		if err != nil {
			failureMessages = append(failureMessages, fmt.Sprintf("node/%s stuck with no details because: %v", stuckNode.Name, err))
			continue
		}
		for podKey, reason := range interestingPods {
			failureMessages = append(failureMessages, fmt.Sprintf("node/%s stuck because pod/%s -n %s %v", stuckNode.Name, podKey.name, podKey.namespace, reason))
		}
		if len(failureMessages) > 100 {
			break
		}
	}
	// sort for stable condition
	sort.Strings(failureMessages)

	// use first 100 messages to avoid blowing out size
	if len(failureMessages) > 100 {
		failureMessages = failureMessages[:100]
	}
	statusMessage := strings.Join(failureMessages, "; ")
	// TODO no need if we take this and wire up a client.
	logger.Info(statusMessage)

	// use real generated client to set only the StuckNode condition

	return nil
}

type podIdentifier struct {
	namespace string
	name      string
}

// interestingPodsOnStuckNode returns pod namespace,name tuples with a reason why they are interesting
// Common values are
// 1. Pod has very long graceful termination
// 2. Pod has PDB preventing eviction
func interestingPodsOnStuckNode(ctx context.Context, kubeClient kubernetes.Interface, node corev1.Node, drainTimeoutDuration time.Duration) (map[podIdentifier]error, error) {
	allPodsOnNode, err := kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + node.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	podToReason := make(map[podIdentifier]error)
	for _, pod := range allPodsOnNode.Items {
		nodeDrainWillNotWaitForPod := false
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "DaemonSet" {
				nodeDrainWillNotWaitForPod = true
			}
		}
		if nodeDrainWillNotWaitForPod {
			continue
		}
		if isPodFinishedRunning(pod) {
			continue
		}

		podKey := podIdentifier{
			namespace: pod.Namespace,
			name:      pod.Name,
		}
		switch {
		case pod.DeletionTimestamp == nil:
			// check to see if the pod is protected by a PDB that prevents deletion
			if reasons := checkIfPodIsEvictable(ctx, kubeClient, pod); len(reasons) > 0 {
				podToReason[podKey] = err
			}
		case pod.DeletionTimestamp != nil:
			// check to see if the pod has a really long grace period
			if err := checkIfPodHasLongGracePeriod(ctx, pod, drainTimeoutDuration); err != nil {
				podToReason[podKey] = err
			}
		}
	}

	return podToReason, nil
}

func checkIfPodHasLongGracePeriod(ctx context.Context, pod corev1.Pod, drainTimeoutDuration time.Duration) error {
	if pod.Spec.TerminationGracePeriodSeconds == nil {
		return nil
	}
	maxGracePeriod := int64(0.8 * drainTimeoutDuration.Seconds())
	if *pod.Spec.TerminationGracePeriodSeconds > maxGracePeriod {
		return fmt.Errorf("termination grace period too large: limit=%vs actual=%vs", maxGracePeriod, *pod.Spec.TerminationGracePeriodSeconds)
	}

	return nil
}

// checkIfPodIsEvictable reasons why the pod is not evictable
func checkIfPodIsEvictable(ctx context.Context, kubeClient kubernetes.Interface, pod corev1.Pod) []error {
	// terminal pods are not subject to PDBs
	if canIgnorePDB(pod) {
		return nil
	}

	// TODO option on caching, might not matter
	allPDBsInNamespace, err := kubeClient.PolicyV1().PodDisruptionBudgets(pod.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return []error{fmt.Errorf("failed to list PDBs: %w", err)}
	}

	// logic here is borrowed from the pod eviction REST storage
	matchingPDBs := []policyv1.PodDisruptionBudget{}
	for _, pdb := range allPDBsInNamespace.Items {
		selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err != nil {
			return []error{fmt.Errorf("failed to get parse selector: %w", err)}
		}
		if selector.Matches(labels.Set(pod.Labels)) {
			matchingPDBs = append(matchingPDBs, pdb)
		}
	}

	blockingPDBs := map[string]error{}
	for _, pdb := range matchingPDBs {
		// logic here is borrowed from the pod eviction REST storage

		// isPodReady is the current implementation of IsHealthy
		// If the pod is healthy, it should be guarded by the PDB.
		if !isPodReady(pod) {
			if pdb.Spec.UnhealthyPodEvictionPolicy != nil && *pdb.Spec.UnhealthyPodEvictionPolicy == policyv1.AlwaysAllow {
				// in this case, the unhealthy pod can be evicted, the PDB does not block
				continue
			}
			// default nil and IfHealthyBudget policy
			if pdb.Status.CurrentHealthy >= pdb.Status.DesiredHealthy && pdb.Status.DesiredHealthy > 0 {
				// in this case, the unhealthy pod can be evicted, the PDB does not block
				continue
			}
		}
		if pdb.Status.ObservedGeneration < pdb.Generation {
			blockingPDBs[pdb.Name] = fmt.Errorf("too many changes to PDB to check")
			continue
		}
		if pdb.Status.DisruptionsAllowed < 0 {
			blockingPDBs[pdb.Name] = fmt.Errorf("pdb disruptions allowed is negative")
			continue
		}
		if len(pdb.Status.DisruptedPods) > MaxDisruptedPodSize {
			blockingPDBs[pdb.Name] = fmt.Errorf("DisruptedPods map too big - too many evictions not confirmed by PDB controller")
			continue
		}
		if pdb.Status.DisruptionsAllowed == 0 {
			err := errors.NewTooManyRequests("Cannot evict pod as it would violate the pod's disruption budget.", 0)
			blockingPDBs[pdb.Name] = fmt.Errorf("%v The disruption budget %s needs %d healthy pods and has %d currently", err, pdb.Name, pdb.Status.DesiredHealthy, pdb.Status.CurrentHealthy)
			continue
		}
	}
	if len(blockingPDBs) == 0 {
		return nil
	}

	reasons := []error{}
	for pdbName, reason := range blockingPDBs {
		reasons = append(reasons, fmt.Errorf("pdb/%v reports %v", pdbName, reason))
	}

	return reasons
}

func isPodFinishedRunning(pod corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return true
	}
	return false
}

// some helper functions lifted from kube

// canIgnorePDB returns true for pod conditions that allow the pod to be deleted
// without checking PDBs.
func canIgnorePDB(pod corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed ||
		pod.Status.Phase == corev1.PodPending || !pod.ObjectMeta.DeletionTimestamp.IsZero() {
		return true
	}
	return false
}

// MaxDisruptedPodSize is the max size of PodDisruptionBudgetStatus.DisruptedPods. API server eviction
// subresource handler will refuse to evict pods covered by the corresponding PDB
// if the size of the map exceeds this value. It means a large number of
// evictions have been approved by the API server but not noticed by the PDB controller yet.
// This situation should self-correct because the PDB controller removes
// entries from the map automatically after the PDB DeletionTimeout regardless.
const MaxDisruptedPodSize = 2000

// isPodReady returns true if a pod is ready; false otherwise.
func isPodReady(pod corev1.Pod) bool {
	return isPodReadyConditionTrue(pod.Status)
}

// isPodReadyConditionTrue returns true if a pod is ready; false otherwise.
func isPodReadyConditionTrue(status corev1.PodStatus) bool {
	condition := getPodReadyCondition(status)
	return condition != nil && condition.Status == corev1.ConditionTrue
}

// getPodReadyCondition extracts the pod ready condition from the given status and returns that.
// Returns nil if the condition is not present.
func getPodReadyCondition(status corev1.PodStatus) *corev1.PodCondition {
	_, condition := getPodCondition(&status, corev1.PodReady)
	return condition
}

// getPodCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func getPodCondition(status *corev1.PodStatus, conditionType corev1.PodConditionType) (int, *corev1.PodCondition) {
	if status == nil {
		return -1, nil
	}
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return i, &status.Conditions[i]
		}
	}
	return -1, nil
}

package drain

import (
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"

	"github.com/openshift/managed-upgrade-operator/pkg/pod"
)

func isPdbPod(pdbList *policyv1beta1.PodDisruptionBudgetList) pod.PodPredicate {
	return func(p corev1.Pod) bool {
		return containsMatchLabel(p, pdbList)
	}
}

func isNotPdbPod(pdbList *policyv1beta1.PodDisruptionBudgetList) pod.PodPredicate {
	return func(p corev1.Pod) bool {
		return !containsMatchLabel(p, pdbList)
	}
}

func isOnNode(node *corev1.Node) pod.PodPredicate {
	return func(p corev1.Pod) bool {
		return p.Spec.NodeName == node.Name
	}
}

func isDaemonSet(pod corev1.Pod) bool {
	isDaemonSet := false
	if len(pod.OwnerReferences) > 0 {
		for _, OwnerRef := range pod.OwnerReferences {
			if OwnerRef.Kind == "DaemonSet" {
				isDaemonSet = true
			}
		}
	}

	return isDaemonSet
}

func isNotDaemonSet(pod corev1.Pod) bool {
	return !isDaemonSet(pod)
}

func containsMatchLabel(p corev1.Pod, pdbList *policyv1beta1.PodDisruptionBudgetList) bool {
	isPdbPod := false
	for _, pdb := range pdbList.Items {
		for mlKey, mlValue := range pdb.Spec.Selector.MatchLabels {
			lValue, ok := p.Labels[mlKey]
			if ok && lValue == mlValue {
				isPdbPod = true
				break
			}
		}
	}

	return isPdbPod
}

func hasFinalizers(p corev1.Pod) bool {
	return len(p.ObjectMeta.GetFinalizers()) > 0
}

func hasNoFinalizers(p corev1.Pod) bool {
	return len(p.ObjectMeta.GetFinalizers()) == 0
}

func isTerminating(p corev1.Pod) bool {
	return p.DeletionTimestamp != nil
}

package drain

import (
	"github.com/openshift/managed-upgrade-operator/pkg/pod"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"regexp"
)

func isPdbPod(pdbList *policyv1.PodDisruptionBudgetList) pod.PodPredicate {
	return func(p corev1.Pod) bool {
		return containsMatchLabel(p, pdbList)
	}
}

func isNotPdbPod(pdbList *policyv1.PodDisruptionBudgetList) pod.PodPredicate {
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

func containsMatchLabel(p corev1.Pod, pdbList *policyv1.PodDisruptionBudgetList) bool {
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
	return len(p.GetFinalizers()) > 0
}

func hasNoFinalizers(p corev1.Pod) bool {
	return len(p.GetFinalizers()) == 0
}

func isTerminating(p corev1.Pod) bool {
	return p.DeletionTimestamp != nil
}

func isAllowedNamespace(ignoredNamespacePatterns []string) pod.PodPredicate {
	return func(p corev1.Pod) bool {
		return containsIgnoredNamespace(p, ignoredNamespacePatterns)
	}
}

func containsIgnoredNamespace(p corev1.Pod, ignoredNamespacePatterns []string) bool {
	for _, nsPattern := range ignoredNamespacePatterns {
		rxp := regexp.MustCompile(nsPattern)
		if rxp.MatchString(p.Namespace) {
			return false
		}
	}
	return true
}

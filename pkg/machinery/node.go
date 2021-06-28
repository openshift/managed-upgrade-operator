package machinery

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IsCordonedResult is a type that holds cordoned information
type IsCordonedResult struct {
	IsCordoned bool
	AddedAt    *metav1.Time
}

// IsNodeCordoned returns a IsNodeCordoned result
func (m *machinery) IsNodeCordoned(node *corev1.Node) *IsCordonedResult {
	var cordonAddedTime *metav1.Time
	isCordoned := false
	if node.Spec.Unschedulable && len(node.Spec.Taints) > 0 {
		for _, n := range node.Spec.Taints {
			if n.Effect == corev1.TaintEffectNoSchedule {
				isCordoned = true
				cordonAddedTime = n.TimeAdded
			}
		}
	}

	return &IsCordonedResult{
		IsCordoned: isCordoned,
		AddedAt:    cordonAddedTime,
	}
}

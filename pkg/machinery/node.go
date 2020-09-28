package machinery

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type IsDrainResult struct {
	IsDraining bool
	StartTime  *metav1.Time
}

func (m *machinery) IsNodeDraining(node *corev1.Node) *IsDrainResult {
	var drainStartedAtTime *metav1.Time
	isDraining := false
	if node.Spec.Unschedulable && len(node.Spec.Taints) > 0 {
		for _, n := range node.Spec.Taints {
			if n.Effect == corev1.TaintEffectNoSchedule {
				isDraining = true
				drainStartedAtTime = n.TimeAdded
			}
		}
	}

	return &IsDrainResult{
		IsDraining: isDraining,
		StartTime:  drainStartedAtTime,
	}
}

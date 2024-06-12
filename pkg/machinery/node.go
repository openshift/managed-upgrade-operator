package machinery

import (
	mcoconst "github.com/openshift/machine-config-operator/pkg/daemon/constants"
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
			if n.Effect == corev1.TaintEffectNoSchedule && n.Key == corev1.TaintNodeUnschedulable {
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

// IsNodeUpgrading returns bool
func (m *machinery) IsNodeUpgrading(node *corev1.Node) bool {
	if node.Annotations[mcoconst.MachineConfigDaemonStateAnnotationKey] == mcoconst.MachineConfigDaemonStateWorking {
		return true
	} else {
		return false
	}
}

func (m *machinery) HasMemoryPressure(node *corev1.Node) bool {
	if len(node.Spec.Taints) > 0 {
		// Only check if there are taints
		for _, n := range node.Spec.Taints {
			if n.Effect == corev1.TaintEffectNoSchedule && n.Key == corev1.TaintNodeMemoryPressure {
				return true
			}
		}
	}
	return false
}

func (m *machinery) HasDiskPressure(node *corev1.Node) bool {
	if len(node.Spec.Taints) > 0 {
		// Only check if there are taints
		for _, n := range node.Spec.Taints {
			if n.Effect == corev1.TaintEffectNoSchedule && n.Key == corev1.TaintNodeDiskPressure {
				return true
			}
		}
	}
	return false
}

func (m *machinery) HasPidPressure(node *corev1.Node) bool {
	if len(node.Spec.Taints) > 0 {
		// Only check if there are taints
		for _, n := range node.Spec.Taints {
			if n.Effect == corev1.TaintEffectNoSchedule && n.Key == corev1.TaintNodePIDPressure {
				return true
			}
		}
	}
	return false
}

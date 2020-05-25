package upgradeconfig

import (
	"github.com/go-logr/logr"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

//TODO
func (r *ReconcileUpgradeConfig) updateStatusPending(eqLogger logr.Logger, u *upgradev1alpha1.UpgradeConfig) error {

	return nil

}

func newUpgradeCondition(reason, msg string, conditionType upgradev1alpha1.UpgradeConditionType, s corev1.ConditionStatus) *upgradev1alpha1.UpgradeCondition {
	return &upgradev1alpha1.UpgradeCondition{
		Type:    conditionType,
		Status:  s,
		Reason:  reason,
		Message: msg,
	}
}

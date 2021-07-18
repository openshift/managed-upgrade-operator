package upgradesteps

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// UpgradeStep is the interface for steps that the upgrade runner
// can execute.
type UpgradeStep interface {
	run(ctx context.Context, logger logr.Logger) (bool, error)
	String() string
}

// Run executes the provided steps in order until one fails or all steps
// are completed. The function returns an indication of the last-completed
// UpgradePhase, the success condition of that phase, and any associated error.
func Run(ctx context.Context, logger logr.Logger, steps []UpgradeStep) (upgradev1alpha1.UpgradePhase, *upgradev1alpha1.UpgradeCondition, error) {
	for _, step := range steps {
		logger.Info(fmt.Sprintf("running step %s", step))
		result, err := step.run(ctx, logger)

		if err != nil {
			logger.Error(err, fmt.Sprintf("error when %s", step.String()))
			condition := newUpgradeCondition(fmt.Sprintf("%s not done", step.String()), err.Error(), upgradev1alpha1.UpgradeConditionType(step.String()), corev1.ConditionFalse)
			return upgradev1alpha1.UpgradePhaseUpgrading, condition, err
		}

		if !result {
			logger.Info(fmt.Sprintf("%s not done, skip following steps", step.String()))
			condition := newUpgradeCondition(fmt.Sprintf("%s not done", step.String()), fmt.Sprintf("%s still in progress", step.String()), upgradev1alpha1.UpgradeConditionType(step.String()), corev1.ConditionFalse)
			return upgradev1alpha1.UpgradePhaseUpgrading, condition, nil
		}
	}
	step := steps[len(steps)-1]
	condition := newUpgradeCondition(fmt.Sprintf("%s done", step.String()), fmt.Sprintf("%s is completed", step.String()), upgradev1alpha1.UpgradeConditionType(step.String()), corev1.ConditionTrue)
	return upgradev1alpha1.UpgradePhaseUpgraded, condition, nil
}

// newUpgradeCondition is a helper function for creating and returning
// an UpgradeCondition
func newUpgradeCondition(reason, msg string, conditionType upgradev1alpha1.UpgradeConditionType, s corev1.ConditionStatus) *upgradev1alpha1.UpgradeCondition {
	return &upgradev1alpha1.UpgradeCondition{
		Type:    conditionType,
		Status:  s,
		Reason:  reason,
		Message: msg,
	}
}


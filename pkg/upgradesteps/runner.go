package upgradesteps

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

// UpgradeStep is the interface for steps that the upgrade runner
// can execute.
type UpgradeStep interface {
	run(ctx context.Context, logger logr.Logger) (bool, error)
	String() string
}

// Run executes the provided steps in order until one fails or all steps
// are completed. The function returns an indication of the last-completed
// UpgradePhase any associated error.
func Run(ctx context.Context, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger, steps []UpgradeStep) (upgradev1alpha1.UpgradePhase, error) {
	for _, step := range steps {
		logger.Info(fmt.Sprintf("running step %s", step))
		setConditionStart(step, upgradeConfig)
		result, err := step.run(ctx, logger)

		if err != nil {
			logger.Error(err, fmt.Sprintf("error when %s", step.String()))
			setConditionInProgress(step, err.Error(), upgradeConfig)
			return upgradev1alpha1.UpgradePhaseUpgrading, err
		}

		if !result {
			logger.Info(fmt.Sprintf("%s not done, skip following steps", step.String()))
			setConditionInProgress(step, fmt.Sprintf("%s still in progress", step.String()), upgradeConfig)
			return upgradev1alpha1.UpgradePhaseUpgrading, nil
		}

		setConditionComplete(step, upgradeConfig)
	}
	return upgradev1alpha1.UpgradePhaseUpgraded, nil
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

// setConditionStart adds an UpgradeCondition to the UpgradeConfig indicating
// that a given step has commenced execution.
// If the UpgradeCondition already exists, no action is taken.
func setConditionStart(step UpgradeStep, upgradeConfig *upgradev1alpha1.UpgradeConfig) {
	history := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
	c := history.Conditions.GetCondition(upgradev1alpha1.UpgradeConditionType(step.String()))
	// Only set the condition if it doesn't already exist - the start time should already appear
	if c == nil {
		condition := newUpgradeCondition(fmt.Sprintf("%s not done", step.String()),
			fmt.Sprintf("%s has started", step.String()),
			upgradev1alpha1.UpgradeConditionType(step.String()),
			corev1.ConditionFalse)
		condition.StartTime = &metav1.Time{Time: time.Now()}
		history.Conditions.SetCondition(*condition)
		upgradeConfig.Status.History.SetHistory(*history)
	}
}

// setConditionInProgress adds or updates an UpgradeCondition in the UpgradeConfig indicating
// that a given step is currently executing.
func setConditionInProgress(step UpgradeStep, message string, upgradeConfig *upgradev1alpha1.UpgradeConfig) {
	history := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
	c := history.Conditions.GetCondition(upgradev1alpha1.UpgradeConditionType(step.String()))
	if c != nil {
		c.Message = message
		c.Status = corev1.ConditionFalse
		// Reset completion time because some steps can fail after an earlier success
		c.CompleteTime = nil
		history.Conditions.SetCondition(*c)
		upgradeConfig.Status.History.SetHistory(*history)
	}
}

// setConditionComplete adds or updates an UpgradeCondition in the UpgradeConfig indicating
// that a given step has completed.
func setConditionComplete(step UpgradeStep, upgradeConfig *upgradev1alpha1.UpgradeConfig) {
	history := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
	c := history.Conditions.GetCondition(upgradev1alpha1.UpgradeConditionType(step.String()))
	if c != nil {
		c.Reason = fmt.Sprintf("%s done", step.String())
		c.Message = fmt.Sprintf("%s is completed", step.String())
		c.Status = corev1.ConditionTrue
		// Only set completion time if it isn't already set
		if c.CompleteTime == nil {
			c.CompleteTime = &metav1.Time{Time: time.Now()}
		}
		history.Conditions.SetCondition(*c)
		upgradeConfig.Status.History.SetHistory(*history)
	}
}
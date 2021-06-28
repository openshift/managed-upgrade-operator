package eventmanager

import (
	"fmt"

	"github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/notifier"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// UPGRADE_PRECHECK_FAILED_DESC describes the upgrade pre check failure
	UPGRADE_PRECHECK_FAILED_DESC = "Cluster upgrade to version %s was cancelled as the cluster did not pass its pre-upgrade verification checks. Automated upgrades will be retried on their next scheduling cycle. If you have manually scheduled an upgrade instead, it must now be rescheduled."
	// UPGRADE_PREHEALTHCHECK_FAILED_DESC describes the upgrade pre health check failure
	UPGRADE_PREHEALTHCHECK_FAILED_DESC = "Cluster upgrade to version %s was cancelled during the Pre-Health Check step. Health alerts are firing in the cluster which could impact the upgrade's operation, so the upgrade did not proceed. Automated upgrades will be retried on their next scheduling cycle. If you have manually scheduled an upgrade instead, it must now be rescheduled."
	// UPGRADE_EXTDEPCHECK_FAILED_DESC describes the upgrade external dependency check failure
	UPGRADE_EXTDEPCHECK_FAILED_DESC = "Cluster upgrade to version %s was cancelled during the External Dependency Availability Check step. A required external dependency of the upgrade was unavailable, so the upgrade did not proceed. Automated upgrades will be retried on their next scheduling cycle. If you have manually scheduled an upgrade instead, it must now be rescheduled."
	// UPGRADE_SCALE_FAILED_DESC describes the upgrade scaling failed
	UPGRADE_SCALE_FAILED_DESC = "Cluster upgrade to version %s was cancelled during the Scale-Up Worker Node step. A temporary additional worker node was unable to be created to temporarily house workloads, so the upgrade did not proceed. Automated upgrades will be retried on their next scheduling cycle. If you have manually scheduled an upgrade instead, it must now be rescheduled."

	// UPGRADE_DEFAULT_DELAY_DESC describes the upgrade default delay
	UPGRADE_DEFAULT_DELAY_DESC = "Cluster upgrade to version %s is experiencing a delay whilst it performs necessary pre-upgrade procedures. The upgrade will continue to retry. This is an informational notification and no action is required."
	// UPGRADE_PREHEALTHCHECK_DELAY_DESC describes the upgrade pre health check delay
	UPGRADE_PREHEALTHCHECK_DELAY_DESC = "Cluster upgrade to version %s is experiencing a delay as health alerts are firing in the cluster which could impact the upgrade's operation. The upgrade will continue to retry. This is an informational notification and no action is required by you."
	// UPGRADE_EXTDEPCHECK_DELAY_DESC describes the upgrade external dependency check delay
	UPGRADE_EXTDEPCHECK_DELAY_DESC = "Cluster upgrade to version %s is experiencing a delay as an external dependency of the upgrade is currently unavailable. The upgrade will continue to retry. This is an informational notification and no action is required by you."
	// UPGRADE_SCALE_DELAY_DESC describes the upgrade scaling delayed
	UPGRADE_SCALE_DELAY_DESC = "Cluster upgrade to version %s is experiencing a delay attempting to scale up an additional worker node. The upgrade will continue to retry. This is an informational notification and no action is required by you."
)

// EventManager enables implementation of an EventManager
//go:generate mockgen -destination=mocks/eventmanager.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/eventmanager EventManager
type EventManager interface {
	Notify(state notifier.NotifyState) error
}

// EventManagerBuilder enables implementation of an EventManagerBuilder
//go:generate mockgen -destination=mocks/eventmanager_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/eventmanager EventManagerBuilder
type EventManagerBuilder interface {
	NewManager(client.Client) (EventManager, error)
}

// NewBuilder returns an eventManagerBuilder
func NewBuilder() EventManagerBuilder {
	return &eventManagerBuilder{}
}

type eventManagerBuilder struct{}

type eventManager struct {
	client               client.Client
	notifier             notifier.Notifier
	metrics              metrics.Metrics
	upgradeConfigManager upgradeconfigmanager.UpgradeConfigManager
	configManagerBuilder configmanager.ConfigManagerBuilder
}

func (emb *eventManagerBuilder) NewManager(client client.Client) (EventManager, error) {
	cmBuilder := configmanager.NewBuilder()
	ucb := upgradeconfigmanager.NewBuilder()
	ucm, err := upgradeconfigmanager.NewBuilder().NewManager(client)
	if err != nil {
		return nil, err
	}
	metricsClient, err := metrics.NewBuilder().NewClient(client)
	if err != nil {
		return nil, err
	}
	notifier, err := notifier.NewBuilder().New(client, cmBuilder, ucb)
	if err != nil {
		return nil, err
	}

	return &eventManager{
		client:               client,
		upgradeConfigManager: ucm,
		metrics:              metricsClient,
		notifier:             notifier,
		configManagerBuilder: cmBuilder,
	}, nil
}

func (s *eventManager) Notify(state notifier.NotifyState) error {
	// Get the current UpgradeConfig
	uc, err := s.upgradeConfigManager.Get()
	if err != nil {
		if err == upgradeconfigmanager.ErrUpgradeConfigNotFound {
			return nil
		}
		return fmt.Errorf("unable to find UpgradeConfig: %v", err)
	}

	// Check if a notification for it has been sent successfully - if so, nothing to do
	isNotified, err := s.metrics.IsMetricNotificationEventSentSet(uc.Name, string(state), uc.Spec.Desired.Version)
	if err != nil {
		return fmt.Errorf("can't check cluster metric NotificationSent: %v", err)
	}
	if isNotified {
		return nil
	}

	// Customize the state description
	var description string
	switch state {
	case notifier.StateStarted:
		description = fmt.Sprintf("Cluster is currently being upgraded to version %s", uc.Spec.Desired.Version)
	case notifier.StateDelayed:
		description = createDelayedDescription(uc)
	case notifier.StateCompleted:
		description = fmt.Sprintf("Cluster has been successfully upgraded to version %s", uc.Spec.Desired.Version)
	case notifier.StateFailed:
		description = createFailureDescription(uc)
	default:
		return fmt.Errorf("state %v not yet implemented", state)
	}

	// Send the notification
	err = s.notifier.NotifyState(state, description)
	if err != nil {
		return fmt.Errorf("can't send notification '%s': %v", state, err)
	}
	s.metrics.UpdateMetricNotificationEventSent(uc.Name, string(state), uc.Spec.Desired.Version)

	return nil
}

// Generates a Failure notification description based on the UpgradeConfig's last failed state
func createFailureDescription(uc *v1alpha1.UpgradeConfig) string {
	// Default failure message
	var description = fmt.Sprintf(UPGRADE_PRECHECK_FAILED_DESC, uc.Spec.Desired.Version)

	history := uc.Status.History.GetHistory(uc.Spec.Desired.Version)
	// Handle a missing history
	if history == nil {
		return description
	}
	// Handle no conditions available
	if len(history.Conditions) == 0 {
		return description
	}

	// Find the condition which will describe what step the upgrade got to
	var failedCondition v1alpha1.UpgradeCondition
	foundFailedCondition := false
	for _, condition := range history.Conditions {
		// Find the first incomplete condition (should only be one)
		if condition.IsFalse() {
			failedCondition = condition
			foundFailedCondition = true
			break
		}
	}

	// No incomplete condition? Just return default
	if !foundFailedCondition {
		return description
	}

	switch failedCondition.Type {
	case v1alpha1.UpgradePreHealthCheck:
		description = fmt.Sprintf(UPGRADE_PREHEALTHCHECK_FAILED_DESC, uc.Spec.Desired.Version)
	case v1alpha1.ExtDepAvailabilityCheck:
		description = fmt.Sprintf(UPGRADE_EXTDEPCHECK_FAILED_DESC, uc.Spec.Desired.Version)
	case v1alpha1.UpgradeScaleUpExtraNodes:
		description = fmt.Sprintf(UPGRADE_SCALE_FAILED_DESC, uc.Spec.Desired.Version)
	}

	return description
}

// Generates a Delayed notification description based on the UpgradeConfig's last state
func createDelayedDescription(uc *v1alpha1.UpgradeConfig) string {
	// Default delayed message
	var description = fmt.Sprintf(UPGRADE_DEFAULT_DELAY_DESC, uc.Spec.Desired.Version)

	history := uc.Status.History.GetHistory(uc.Spec.Desired.Version)
	// Handle a missing history
	if history == nil {
		return description
	}
	// Handle no conditions available
	if len(history.Conditions) == 0 {
		return description
	}

	// Find the condition which will describe what step the upgrade got to
	var delayedCondition v1alpha1.UpgradeCondition
	foundDelayedCondition := false
	for _, condition := range history.Conditions {
		// Find the first incomplete condition (should only be one)
		if condition.IsFalse() {
			delayedCondition = condition
			foundDelayedCondition = true
			break
		}
	}

	// No incomplete condition? Just return default
	if !foundDelayedCondition {
		return description
	}

	switch delayedCondition.Type {
	case v1alpha1.UpgradePreHealthCheck:
		description = fmt.Sprintf(UPGRADE_PREHEALTHCHECK_DELAY_DESC, uc.Spec.Desired.Version)
	case v1alpha1.ExtDepAvailabilityCheck:
		description = fmt.Sprintf(UPGRADE_EXTDEPCHECK_DELAY_DESC, uc.Spec.Desired.Version)
	case v1alpha1.UpgradeScaleUpExtraNodes:
		description = fmt.Sprintf(UPGRADE_SCALE_DELAY_DESC, uc.Spec.Desired.Version)
	}

	return description
}

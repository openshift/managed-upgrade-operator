package eventmanager

import (
	"fmt"
	"time"

	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/notifier"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	REFRESH_INTERVAL = 5 * time.Minute
)

//go:generate mockgen -destination=mocks/eventmanager.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/eventmanager EventManager
type EventManager interface {
	Notify(state notifier.NotifyState) error
}

//go:generate mockgen -destination=mocks/eventmanager_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/eventmanager EventManagerBuilder
type EventManagerBuilder interface {
	NewManager(client.Client) (EventManager, error)
}

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
		} else {
			return fmt.Errorf("unable to find UpgradeConfig: %v", err)
		}
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
		description = fmt.Sprintf("Cluster upgrade to version %s is currently delayed", uc.Spec.Desired.Version)
	case notifier.StateCompleted:
		description = fmt.Sprintf("Cluster has been successfully upgraded to version %s", uc.Spec.Desired.Version)
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

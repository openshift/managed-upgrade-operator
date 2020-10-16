package eventmanager

import (
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/notifier"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
)

const (
	REFRESH_INTERVAL = 3 * time.Minute
)

var log = logf.Log.WithName("event-manager")

//go:generate mockgen -destination=mocks/eventmanager.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/eventmanager EventManager
type EventManager interface {
	Start(stopCh <-chan struct{})
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

// Syncs UpgradeConfigs from the spec provider periodically until the operator is killed or a message is sent on the stopCh
func (s *eventManager) Start(stopCh <-chan struct{}) {
	log.Info("Starting the eventManager")

	err := s.notificationRefresh()
	if err != nil {
		log.Error(err, "error during notification refresh")
	}

	for {
		select {
		case <-time.After(REFRESH_INTERVAL):
			err = s.notificationRefresh()
			if err != nil {
				log.Error(err, "error during notification refresh")
			}
		case <-stopCh:
			log.Info("Stopping the eventManager")
			break
		}
	}
}

func (s *eventManager) notificationRefresh() error {

	// Get the current UpgradeConfig
	uc, err := s.upgradeConfigManager.Get()
	if err != nil {
		if err == upgradeconfigmanager.ErrUpgradeConfigNotFound {
			return nil
		} else {
			return fmt.Errorf("unable to find UpgradeConfig: %v", err)
		}
	}

	// Check all the types of notifications we can send
	err = checkUpgradeStart(s.metrics, s.notifier, uc)
	if err != nil {
		return err
	}
	err = checkUpgradeEnd(s.metrics, s.notifier, uc)
	if err != nil {
		return err
	}
	return nil
}

func checkUpgradeStart(mc metrics.Metrics, nc notifier.Notifier, uc *upgradev1alpha1.UpgradeConfig) error {

	upgradeStarted := false

	// Check if the cluster is at the version in the UpgradeConfig
	isSet, err := mc.IsClusterVersionAtVersion(uc.Spec.Desired.Version)
	if err != nil {
		return fmt.Errorf("can't check cluster metric ClusterVersion: %v", err)
	} else {
		upgradeStarted = isSet
	}

	// As a backup if metrics is unavailable, check if upgradeConfig indicates the upgrade has completed
	if !upgradeStarted {
		upgradePhase, err := getUpgradePhase(uc)
		if err == nil && *upgradePhase == upgradev1alpha1.UpgradePhaseUpgraded {
			upgradeStarted = true
		}
	}

	if !upgradeStarted {
		return nil
	}

	// Check if a notification for it has already been sent successfully
	isNotified, err := mc.IsMetricNotificationEventSentSet(uc.Name, string(notifier.StateStarted), uc.Spec.Desired.Version)
	if err != nil {
		return fmt.Errorf("can't check cluster metric NotificationSent: %v", err)
	}
	if isNotified {
		return nil
	}

	// Send the notification and indicate it has been sent
	description := fmt.Sprintf("Cluster is currently being upgraded to version %s", uc.Spec.Desired.Version)
	err = nc.NotifyState(notifier.StateStarted, description)
	if err != nil {
		return fmt.Errorf("can't send notification '%s': %v", notifier.StateStarted, err)
	}
	mc.UpdateMetricNotificationEventSent(uc.Name, string(notifier.StateStarted), uc.Spec.Desired.Version)

	return nil
}

func checkUpgradeEnd(mc metrics.Metrics, nc notifier.Notifier, uc *upgradev1alpha1.UpgradeConfig) error {

	upgradeEnded := false

	// Check metrics if the node upgrade end time metric has been set
	isSet, err := mc.IsMetricNodeUpgradeEndTimeSet(uc.Name, uc.Spec.Desired.Version)
	if err != nil {
		log.Error(err, "could not check metrics for upgrade end time")
	} else {
	   upgradeEnded = isSet
	}

	// As a backup, check if upgradeConfig indicates the upgrade has completed
	if !upgradeEnded {
		upgradePhase, err := getUpgradePhase(uc)
		if err == nil && *upgradePhase == upgradev1alpha1.UpgradePhaseUpgraded {
			upgradeEnded = true
		}
	}

	// If the upgrade hasn't ended, do nothing
	if !upgradeEnded {
		return nil
	}

	// Check if a notification for it has been sent successfully
	isNotified, err := mc.IsMetricNotificationEventSentSet(uc.Name, string(notifier.StateCompleted), uc.Spec.Desired.Version)
	if err != nil {
		return fmt.Errorf("can't check cluster metric NotificationSent: %v", err)
	}
	if isNotified {
		return nil
	}

	// Send the notification and indicate it has been sent
	description := fmt.Sprintf("Cluster has been successfully upgraded to version %s", uc.Spec.Desired.Version)
	err = nc.NotifyState(notifier.StateCompleted, description)
	if err != nil {
		return fmt.Errorf("can't send notification '%s': %v", notifier.StateCompleted, err)
	}
	mc.UpdateMetricNotificationEventSent(uc.Name, string(notifier.StateCompleted), uc.Spec.Desired.Version)

	return nil
}

// Returns the phase of the currently executing upgrade or error if no phase can be found
func getUpgradePhase(uc *upgradev1alpha1.UpgradeConfig) (*upgradev1alpha1.UpgradePhase, error) {
	// Check if upgradeConfig indicates the upgrade has completed
	var history upgradev1alpha1.UpgradeHistory
	found := false
	for _, h := range uc.Status.History {
		if h.Version == uc.Spec.Desired.Version {
			history = h
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("no history found")
	}

	return &history.Phase, nil
}
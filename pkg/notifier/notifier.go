package notifier

import (
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/config"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
)

// Notifier is an interface that enables implementation of a Notifier
//
//go:generate mockgen -destination=mocks/notifier.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/notifier Notifier
type Notifier interface {
	NotifyState(value MuoState, description string) error
}

// NotifierBuilder is an interface that enables implementation of a NotifierBuilder
//
//go:generate mockgen -destination=mocks/notifier_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/notifier NotifierBuilder
type NotifierBuilder interface {
	New(client.Client, configmanager.ConfigManagerBuilder, upgradeconfigmanager.UpgradeConfigManagerBuilder) (Notifier, error)
}

// Represents valid notify states that can be reported
const (
	MuoStatePending                       MuoState = "StatePending"
	MuoStateStarted                       MuoState = "StateStarted"
	MuoStateCompleted                     MuoState = "StateCompleted"
	MuoStateDelayed                       MuoState = "StateDelayed"
	MuoStateFailed                        MuoState = "StateFailed"
	MuoStateCancelled                     MuoState = "StateCancelled"
	MuoStateScheduled                     MuoState = "StateScheduled"
	MuoStateScaleSkipped                  MuoState = "StateScaleSkipped"
	MuoStateSkipped                       MuoState = "StateSkipped"
	MuoStateHealthCheck                   MuoState = "StateHealthCheck"
	MuoStateControlPlaneUpgradeStartedSL  MuoState = "StateControlPlaneStartedSL"
	MuoStateControlPlaneUpgradeFinishedSL MuoState = "StateControlPlaneFinishedSL"
	MuoStateWorkerPlaneUpgradeFinishedSL  MuoState = "StateWorkerPlaneFinishedSL"
)

// MuoState is a type
type MuoState string

// Errors
var (
	ErrNoNotifierConfigured = fmt.Errorf("no valid configured notifier")
)

// NewBuilder creates a new Notifier instance builder
func NewBuilder() NotifierBuilder {
	return &notifierBuilder{}
}

type notifierBuilder struct{}

// Creates a new Notifier instance
func (nb *notifierBuilder) New(client client.Client, cfgBuilder configmanager.ConfigManagerBuilder, upgradeConfigManagerBuilder upgradeconfigmanager.UpgradeConfigManagerBuilder) (Notifier, error) {
	cfg, err := readNotifierConfig(client, cfgBuilder)
	if err != nil {
		return nil, err
	}

	// Initialise upgrade config manager
	upgradeConfigManager, err := upgradeConfigManagerBuilder.NewManager(client)
	if err != nil {
		return nil, err
	}

	switch strings.ToUpper(cfg.ConfigManager.Source) {
	case "OCM":
		cfg, err := readOcmNotifierConfig(client, cfgBuilder)
		if err != nil {
			return nil, err
		}
		mgr, err := NewOCMNotifier(client, cfg.GetOCMBaseURL(), upgradeConfigManager)
		if err != nil {
			return nil, err
		}
		return mgr, nil
	default:
		// Create a log notifier as a fallback
		mgr, err := NewLogNotifier()
		return mgr, err
	}
}

// Read notifier configuration
func readNotifierConfig(client client.Client, cfb configmanager.ConfigManagerBuilder) (*NotifierConfig, error) {
	cfg := &NotifierConfig{}

	target := config.CMTarget{}
	cmTarget, err := target.NewCMTarget()
	if err != nil {
		return cfg, err
	}

	cfm := cfb.New(client, cmTarget)
	err = cfm.Into(cfg)
	if err != nil {
		return cfg, err
	}
	return cfg, cfg.IsValid()
}

// Read OCM provider configuration
func readOcmNotifierConfig(client client.Client, cfb configmanager.ConfigManagerBuilder) (*OcmNotifierConfig, error) {
	cfg := &OcmNotifierConfig{}

	target := config.CMTarget{}
	cmTarget, err := target.NewCMTarget()
	if err != nil {
		return cfg, err
	}

	cfm := cfb.New(client, cmTarget)
	err = cfm.Into(cfg)
	if err != nil {
		return cfg, err
	}
	return cfg, cfg.IsValid()
}

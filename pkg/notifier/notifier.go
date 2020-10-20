package notifier

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
	"github.com/openshift/managed-upgrade-operator/util"
)

//go:generate mockgen -destination=mocks/notifier.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/notifier Notifier
type Notifier interface {
	NotifyState(value NotifyState, description string) error
}

//go:generate mockgen -destination=mocks/notifier_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/notifier NotifierBuilder
type NotifierBuilder interface {
	New(client.Client, configmanager.ConfigManagerBuilder, upgradeconfigmanager.UpgradeConfigManagerBuilder) (Notifier, error)
}

// Represents valid notify states that can be reported
const (
	StatePending   NotifyState = "pending"
	StateStarted   NotifyState = "started"
	StateCompleted NotifyState = "completed"
	StateDelayed   NotifyState = "delayed"
	StateFailed    NotifyState = "failed"
)

type NotifyState string

// Errors
var (
	ErrNoNotifierConfigured = fmt.Errorf("no valid configured notifier")
)

// Creates a new Notifier instance builder
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

	switch cfg.ConfigManager.Source {
	case "OCM":
		cfg, err := readOcmNotifierConfig(client, cfgBuilder)
		if err != nil {
			return nil, err
		}
		mgr, err := New(client, cfg.GetOCMBaseURL(), upgradeConfigManager)
		if err != nil {
			return nil, err
		}
		return mgr, nil
	}
	return nil, ErrNoNotifierConfigured
}

// Read notifier configuration
func readNotifierConfig(client client.Client, cfb configmanager.ConfigManagerBuilder) (*NotifierConfig, error) {
	ns, err := util.GetOperatorNamespace()
	if err != nil {
		return nil, err
	}
	cfm := cfb.New(client, ns)
	cfg := &NotifierConfig{}
	err = cfm.Into(cfg)
	if err != nil {
		return nil, err
	}
	return cfg, cfg.IsValid()
}

// Read OCM provider configuration
func readOcmNotifierConfig(client client.Client, cfb configmanager.ConfigManagerBuilder) (*OcmNotifierConfig, error) {
	// Fetch the provider config
	ns, err := util.GetOperatorNamespace()
	if err != nil {
		return nil, err
	}
	cfm := cfb.New(client, ns)
	cfg := &OcmNotifierConfig{}
	err = cfm.Into(cfg)
	if err != nil {
		return nil, err
	}
	return cfg, cfg.IsValid()
}

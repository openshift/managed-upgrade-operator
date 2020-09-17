package upgrade_config_manager

import (
	"fmt"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/pkg/ocm_upgrade_config_manager"
)

type ConfigManagerSource string

//go:generate mockgen -destination=mocks/upgrade_config_manager.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/upgrade_config_manager UpgradeConfigManager
type UpgradeConfigManager interface {
	Start(stopCh <-chan struct{})
	RefreshUpgradeConfig() (changed bool, err error)
}

//go:generate mockgen -destination=mocks/upgrade_config_manager_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/upgrade_config_manager UpgradeConfigManagerBuilder
type UpgradeConfigManagerBuilder interface {
	NewManager(client.Client) (UpgradeConfigManager, error)
}

func NewBuilder() UpgradeConfigManagerBuilder {
	return &upgradeConfigManagerBuilder{}
}

type upgradeConfigManagerBuilder struct{}

func (ucb *upgradeConfigManagerBuilder) NewManager(client client.Client) (UpgradeConfigManager, error) {

	cfg, err := readConfigManagerConfig(client)
	if err != nil {
		return nil, err
	}
	err = cfg.IsValid()
	if err != nil {
		return nil, err
	}

	switch cfg.ConfigManager.Source {
	case string(OCM):
		mgr, err := ocm_upgrade_config_manager.NewManager(client)
		if err != nil {
			return nil, err
		}
		return mgr, nil
	default:
		return nil, fmt.Errorf("unhandled UpgradeConfig Manager type: %v", cfg.ConfigManager.Source)
	}
}

func readConfigManagerConfig(client client.Client) (*upgradeConfigManagerConfig, error) {
	ns, err := getOperatorNamespace()
	if err != nil {
		return nil, err
	}

	cfm := configmanager.NewBuilder().New(client, ns)
	cfg := &upgradeConfigManagerConfig{}
	err = cfm.Into(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func getOperatorNamespace() (string, error) {
	envVarOperatorNamespace := "OPERATOR_NAMESPACE"
	ns, found := os.LookupEnv(envVarOperatorNamespace)
	if !found {
		return "", fmt.Errorf("%s must be set", envVarOperatorNamespace)
	}
	return ns, nil
}

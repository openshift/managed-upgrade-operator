package policyprovider

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/ocmprovider"
	"github.com/openshift/managed-upgrade-operator/util"
)

//go:generate mockgen -destination=mocks/policyprovider.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/policyprovider PolicyProvider
type PolicyProvider interface {
	Get() ([]upgradev1alpha1.UpgradeConfigSpec, error)
}

//go:generate mockgen -destination=mocks/policyprovider_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/policyprovider PolicyProviderBuilder
type PolicyProviderBuilder interface {
	New(client.Client, configmanager.ConfigManagerBuilder) (PolicyProvider, error)
}

func NewBuilder() PolicyProviderBuilder {
	return &policyProviderBuilder{}
}

type policyProviderBuilder struct{}

func (ppb *policyProviderBuilder) New(client client.Client, builder configmanager.ConfigManagerBuilder) (PolicyProvider, error) {
	cfg, err := readPolicyProviderConfig(client, builder)
	if err != nil {
		return nil, err
	}

	switch cfg.ConfigManager.Source {
	case "OCM":
		cfg, err := readOcmProviderConfig(client, builder)
		if err != nil {
			return nil, err
		}
		mgr, err := ocmprovider.New(client, cfg.GetOCMBaseURL())
		if err != nil {
			return nil, err
		}
		return mgr, nil
	}
	return nil, fmt.Errorf("no valid configured policy provider")
}

// Read policy provider configuration
func readPolicyProviderConfig(client client.Client, cfb configmanager.ConfigManagerBuilder) (*PolicyProviderConfig, error) {
	ns, err := util.GetOperatorNamespace()
	if err != nil {
		return nil, err
	}
	cfm := cfb.New(client, ns)
	cfg := &PolicyProviderConfig{}
	err = cfm.Into(cfg)
	if err != nil {
		return nil, err
	}
	return cfg, cfg.IsValid()
}

// Read OCM provider configuration
func readOcmProviderConfig(client client.Client, cfb configmanager.ConfigManagerBuilder) (*ocmprovider.OcmProviderConfig, error) {
	// Fetch the provider config
	ns, err := util.GetOperatorNamespace()
	if err != nil {
		return nil, err
	}
	cfm := cfb.New(client, ns)
	cfg := &ocmprovider.OcmProviderConfig{}
	err = cfm.Into(cfg)
	if err != nil {
		return nil, err
	}
	return cfg, cfg.IsValid()
}
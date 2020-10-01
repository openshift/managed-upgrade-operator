package specprovider

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/ocmprovider"
	"github.com/openshift/managed-upgrade-operator/util"
)

//go:generate mockgen -destination=mocks/specprovider.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/specprovider SpecProvider
type SpecProvider interface {
	Get() ([]upgradev1alpha1.UpgradeConfigSpec, error)
}

//go:generate mockgen -destination=mocks/specprovider_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/specprovider SpecProviderBuilder
type SpecProviderBuilder interface {
	New(client.Client, configmanager.ConfigManagerBuilder) (SpecProvider, error)
}

func NewBuilder() SpecProviderBuilder {
	return &specProviderBuilder{}
}

type specProviderBuilder struct{}

func (ppb *specProviderBuilder) New(client client.Client, builder configmanager.ConfigManagerBuilder) (SpecProvider, error) {
	cfg, err := readSpecProviderConfig(client, builder)
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
	return nil, fmt.Errorf("no valid configured spec provider")
}

// Read spec provider configuration
func readSpecProviderConfig(client client.Client, cfb configmanager.ConfigManagerBuilder) (*SpecProviderConfig, error) {
	ns, err := util.GetOperatorNamespace()
	if err != nil {
		return nil, err
	}
	cfm := cfb.New(client, ns)
	cfg := &SpecProviderConfig{}
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
package specprovider

import (
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/localprovider"
	"github.com/openshift/managed-upgrade-operator/pkg/ocmprovider"
	"github.com/openshift/managed-upgrade-operator/util"
)

// SpecProvider is an interface that enables an implementation of a spec provider
//go:generate mockgen -destination=mocks/specprovider.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/specprovider SpecProvider
type SpecProvider interface {
	Get() ([]upgradev1alpha1.UpgradeConfigSpec, error)
}

// SpecProviderBuilder is an interface that enables implementation of a spec provider builder
//go:generate mockgen -destination=mocks/specprovider_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/specprovider SpecProviderBuilder
type SpecProviderBuilder interface {
	New(client.Client, configmanager.ConfigManagerBuilder) (SpecProvider, error)
}

// NewBuilder returns a new specProviderBuilder
func NewBuilder() SpecProviderBuilder {
	return &specProviderBuilder{}
}

type specProviderBuilder struct{}

// Errors
var ()

func (ppb *specProviderBuilder) New(client client.Client, builder configmanager.ConfigManagerBuilder) (SpecProvider, error) {
	cfg, err := readSpecProviderConfig(client, builder)
	if err != nil {
		return nil, err
	}

	switch strings.ToUpper(cfg.ConfigManager.Source) {
	case "OCM":
		logf.Log.Logger.Info("Using OCM as the upgrade config provider")
		cfg, err := readOcmProviderConfig(client, builder)
		if err != nil {
			return nil, err
		}
		mgr, err := ocmprovider.New(client, cfg.GetOCMBaseURL())
		if err != nil {
			return nil, err
		}
		return mgr, nil
	case "LOCAL":
		logf.Log.Logger.Info("Using local CR as the upgrade config provider")
		cfg, err := readLocalProviderConfig(client, builder)
		if err != nil {
			return nil, err
		}
		provider, err := localprovider.New(client, cfg.ConfigManager.LocalConfigName)
		if err != nil {
			return nil, err
		}
		return provider, nil
	}
	return nil, ErrInvalidSpecProvider
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

// Read Local Provider configuration
func readLocalProviderConfig(client client.Client, cfb configmanager.ConfigManagerBuilder) (*localprovider.LocalProviderConfig, error) {
	ns, err := util.GetOperatorNamespace()
	if err != nil {
		return nil, err
	}
	cfm := cfb.New(client, ns)
	cfg := &localprovider.LocalProviderConfig{}
	err = cfm.Into(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, cfg.IsValid()
}

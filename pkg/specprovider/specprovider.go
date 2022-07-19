package specprovider

import (
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openshift/managed-upgrade-operator/config"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/localprovider"
	"github.com/openshift/managed-upgrade-operator/pkg/ocmprovider"
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
		logf.Log.Info("Using OCM as the upgrade config provider")
		providerCfg, err := readOcmProviderConfig(client, builder)
		if err != nil {
			return nil, err
		}
		mgr, err := ocmprovider.New(client, cfg.GetUpgradeType(), providerCfg.GetOCMBaseURL())
		if err != nil {
			return nil, err
		}
		return mgr, nil
	case "LOCAL":
		logf.Log.Info("Using local CR as the upgrade config provider")
		providerCfg, err := readLocalProviderConfig(client, builder)
		if err != nil {
			return nil, err
		}
		provider, err := localprovider.New(client, providerCfg.ConfigManager.LocalConfigName)
		if err != nil {
			return nil, err
		}
		return provider, nil
	}
	return nil, ErrInvalidSpecProvider
}

// Read spec provider configuration
func readSpecProviderConfig(client client.Client, cfb configmanager.ConfigManagerBuilder) (*SpecProviderConfig, error) {
	cfg := &SpecProviderConfig{}
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
func readOcmProviderConfig(client client.Client, cfb configmanager.ConfigManagerBuilder) (*ocmprovider.OcmProviderConfig, error) {
	cfg := &ocmprovider.OcmProviderConfig{}

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

// Read Local Provider configuration
func readLocalProviderConfig(client client.Client, cfb configmanager.ConfigManagerBuilder) (*localprovider.LocalProviderConfig, error) {
	cfg := &localprovider.LocalProviderConfig{}

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

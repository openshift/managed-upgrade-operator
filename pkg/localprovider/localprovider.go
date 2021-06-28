package localprovider

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/util"
)

// UPGRADECONFIG_CR_NAME is the name of the Custom Resource that the provider will manage
const UPGRADECONFIG_CR_NAME = "managed-upgrade-config"

var log = logf.Log.WithName("upgradeconfig-localprovider")

// New returns a new localProvider
func New(c client.Client, name string) (*localProvider, error) {
	return &localProvider{
		client:  c,
		cfgname: name,
	}, nil
}

type localProvider struct {
	client  client.Client
	cfgname string
}

// Get checks the upgrade config on the cluster with matched name which is going to perform the upgrade
func (l *localProvider) Get() ([]upgradev1alpha1.UpgradeConfigSpec, error) {
	log.Info("Read the upgrade config from the cluster directly")

	// Get the current UpgradeConfigs on the cluster
	instances, err := fetchUpgradeConfigs(l.client)
	if err != nil {
		return nil, err
	}

	// Get Specs from the upgradeConfig
	specs, err := readSpecFromConfig(*instances)
	if err != nil {
		return nil, err
	}

	return specs, nil
}

// Helper function to extract the spec from the upgradeConfig CR
func readSpecFromConfig(ucl upgradev1alpha1.UpgradeConfigList) ([]upgradev1alpha1.UpgradeConfigSpec, error) {
	upgradeConfigSpecs := make([]upgradev1alpha1.UpgradeConfigSpec, 0)

	for _, u := range ucl.Items {
		// Completed UpgradeConfigs can be ignored
		history := u.Status.History.GetHistory(u.Spec.Desired.Version)
		if history != nil && history.Phase != upgradev1alpha1.UpgradePhaseUpgraded {
			upgradeConfigSpecs = append(upgradeConfigSpecs, u.Spec)
		}
	}
	return upgradeConfigSpecs, nil
}

// Read the CR from cluster with matched name
func fetchUpgradeConfigs(c client.Client) (*upgradev1alpha1.UpgradeConfigList, error) {
	instances := &upgradev1alpha1.UpgradeConfigList{}
	ns, err := util.GetOperatorNamespace()
	if err != nil {
		return nil, err
	}

	err = c.List(context.TODO(), instances, client.InNamespace(ns),
		client.MatchingFields{"metadata.name": UPGRADECONFIG_CR_NAME},
	)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to list the upgrade config with name %s", UPGRADECONFIG_CR_NAME))
		return nil, err
	}

	return instances, nil
}

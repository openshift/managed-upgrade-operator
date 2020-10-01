package upgradeconfigmanager

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/specprovider"
	"github.com/openshift/managed-upgrade-operator/util"
)

type ConfigManagerSource string

var log = logf.Log.WithName("upgrade-config-manager")

const (
	// Name of the Custom Resource that the provider will manage
	UPGRADECONFIG_CR_NAME = "osd-upgrade-config"
	// Jitter factor (percentage / 100) used to alter watch interval
	JITTER_FACTOR = 0.1
)

// Errors
var (
	ErrClusterIsUpgrading       = fmt.Errorf("cluster is upgrading")
	ErrRetrievingUpgradeConfigs = fmt.Errorf("unable to retrieve upgradeconfigs")
	ErrMissingOperatorNamespace = fmt.Errorf("can't determine operator namespace, missing env OPERATOR_NAMESPACE")
	ErrProviderSpecPull         = fmt.Errorf("unable to retrieve upgrade spec")
	ErrRemovingUpgradeConfig    = fmt.Errorf("unable to remove existing UpgradeConfig")
	ErrCreatingUpgradeConfig    = fmt.Errorf("unable to create new UpgradeConfig")
	ErrUpgradeConfigNotFound    = fmt.Errorf("upgrade config not found")
)

//go:generate mockgen -destination=mocks/upgradeconfigmanager.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager UpgradeConfigManager
type UpgradeConfigManager interface {
	Get() (*upgradev1alpha1.UpgradeConfig, error)
	StartSync(stopCh <-chan struct{})
	Refresh() (bool, error)
}

//go:generate mockgen -destination=mocks/upgradeconfigmanager_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager UpgradeConfigManagerBuilder
type UpgradeConfigManagerBuilder interface {
	NewManager(client.Client) (UpgradeConfigManager, error)
}

func NewBuilder() UpgradeConfigManagerBuilder {
	return &upgradeConfigManagerBuilder{}
}

type upgradeConfigManagerBuilder struct{}

type upgradeConfigManager struct {
	client               client.Client
	cvClientBuilder      cv.ClusterVersionBuilder
	specProviderBuilder  specprovider.SpecProviderBuilder
	configManagerBuilder configmanager.ConfigManagerBuilder
}

func (ucb *upgradeConfigManagerBuilder) NewManager(client client.Client) (UpgradeConfigManager, error) {

	spBuilder := specprovider.NewBuilder()
	cvBuilder := cv.NewBuilder()
	cmBuilder := configmanager.NewBuilder()

	return &upgradeConfigManager{
		client:               client,
		cvClientBuilder:      cvBuilder,
		specProviderBuilder:  spBuilder,
		configManagerBuilder: cmBuilder,
	}, nil
}

func (s *upgradeConfigManager) Get() (*upgradev1alpha1.UpgradeConfig, error) {
	instance := &upgradev1alpha1.UpgradeConfig{}
	ns, err := util.GetOperatorNamespace()
	if err != nil {
		return nil, ErrMissingOperatorNamespace
	}
	err = s.client.Get(context.TODO(), client.ObjectKey{Name: UPGRADECONFIG_CR_NAME, Namespace: ns}, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrUpgradeConfigNotFound
		}
		log.Error(err, "error retrieving UpgradeConfig")
		return nil, ErrRetrievingUpgradeConfigs
	}
	return instance, nil
}

// Syncs UpgradeConfigs from the spec provider periodically until the operator is killed or a message is sent on the stopCh
func (s *upgradeConfigManager) StartSync(stopCh <-chan struct{}) {
	log.Info("Starting the upgradeConfigManager")

	// Read manager configuration
	cfg, err := readConfigManagerConfig(s.client, s.configManagerBuilder)
	if err == ErrNoConfigManagerDefined {
		log.Info("No UpgradeConfig manager configuration defined, will not sync")
		return
	}
	if err != nil {
		log.Error(err, "can't read upgradeConfigManager configuration")
		return
	}

	_, err = s.Refresh()
	if err != nil {
		log.Error(err, "unable to refresh upgrade config")
	}

	for {

		// Select a new watch interval with jitter
		duration := durationWithJitter(cfg.GetWatchInterval(), JITTER_FACTOR)

		select {
		case <-time.After(duration):
			_, err := s.Refresh()
			if err != nil {
				log.Error(err, "unable to refresh upgrade config")
			}
		case <-stopCh:
			log.Info("Stopping the upgradeConfigManager")
			break
		}
	}
}

// Refreshes UpgradeConfigs from the UpgradeConfig provider
func (s *upgradeConfigManager) Refresh() (bool, error) {

	// Get the running namespace
	operatorNS, err := util.GetOperatorNamespace()
	if err != nil {
		return false, ErrMissingOperatorNamespace
	}

	// Get the current UpgradeConfigs on the cluster
	currentUpgradeConfig, err := s.Get()
	foundUpgradeConfig := false
	if err != nil {
		if err == ErrUpgradeConfigNotFound {
			currentUpgradeConfig = &upgradev1alpha1.UpgradeConfig{}
		} else {
			return false, err
		}
	} else {
		foundUpgradeConfig = true
	}

	// If we are in the middle of an upgrade, we should not refresh
	cvClient := s.cvClientBuilder.New(s.client)
	upgrading, err := upgradeInProgress(currentUpgradeConfig, cvClient)
	if err != nil {
		return false, err
	}
	if upgrading {
		return false, ErrClusterIsUpgrading
	}

	// Get the latest config specs from the provider
	pp, err := s.specProviderBuilder.New(s.client, s.configManagerBuilder)
	if err != nil {
		return false, fmt.Errorf("unable to create spec provider: %v", err)
	}
	configSpecs, err := pp.Get()
	if err != nil {
		log.Error(err, "error pulling provider specs")
		return false, ErrProviderSpecPull
	}

	// If there are no configSpecs, remove the existing UpgradeConfig
	if len(configSpecs) == 0 {
		if foundUpgradeConfig {
			log.Info(fmt.Sprintf("Removing expired UpgradeConfig %s", currentUpgradeConfig.Name))
			err = s.client.Delete(context.TODO(), currentUpgradeConfig)
			if err != nil {
				log.Error(err, "can't remove UpgradeConfig")
				return false, ErrRemovingUpgradeConfig
			}
			return true, nil
		}
		return false, nil
	}

	// We are basing on an assumption of one (1) UpgradeConfig per cluster right now.
	// So just use the first spec returned
	if len(configSpecs) > 1 {
		log.Info("More than one Upgrade Spec received, only considering the first.")
	}
	upgradeConfigSpec := configSpecs[0]

	// Set up the UpgradeConfig we will replace with
	replacementUpgradeConfig := upgradev1alpha1.UpgradeConfig{}

	// Check if we have an existing UpgradeConfig to compare against, for the refresh
	if foundUpgradeConfig {
		// If there was an existing UpgradeConfig, make a clone of its contents
		currentUpgradeConfig.DeepCopyInto(&replacementUpgradeConfig)
	} else {
		// No existing UpgradeConfig exists, give the new one the default name/namespace
		replacementUpgradeConfig.Name = UPGRADECONFIG_CR_NAME
		replacementUpgradeConfig.Namespace = operatorNS
	}

	// Replace the spec with the refreshed upgrade spec
	upgradeConfigSpec.DeepCopyInto(&replacementUpgradeConfig.Spec)

	// is there a difference between the original and replacement?
	changed := !reflect.DeepEqual(replacementUpgradeConfig.Spec, currentUpgradeConfig.Spec)
	if changed {
		// Apply the resource
		log.Info("cluster upgrade spec has changed, will update")
		err = s.client.Update(context.TODO(), &replacementUpgradeConfig)
		if err != nil {
			if errors.IsNotFound(err) {
				// couldn't update because it didn't exist - create it instead.
				err = s.client.Create(context.TODO(), &replacementUpgradeConfig)
			}
		}
		if err != nil {
			return false, fmt.Errorf("unable to apply UpgradeConfig changes: %v", err)
		}
	} else {
		log.Info(fmt.Sprintf("no change in spec from existing UpgradeConfig %v, won't update", currentUpgradeConfig.Name))
	}

	return changed, nil
}

// Reads the UpgradeConfigManager's configuration
func readConfigManagerConfig(client client.Client, cfb configmanager.ConfigManagerBuilder) (*UpgradeConfigManagerConfig, error) {
	ns, err := util.GetOperatorNamespace()
	if err != nil {
		return nil, err
	}
	cfm := cfb.New(client, ns)
	cfg := &UpgradeConfigManagerConfig{}
	err = cfm.Into(cfg)
	if err != nil {
		return nil, err
	}
	return cfg, cfg.IsValid()
}

// Applies the supplied deviation factor to the given time duration
// and returns the result.
// Adapted from https://github.com/kamilsk/retry/blob/v5/jitter/
func durationWithJitter(t time.Duration, factor float64) time.Duration {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	min := int64(math.Floor(float64(t) * (1 - factor)))
	max := int64(math.Ceil(float64(t) * (1 + factor)))
	return time.Duration(rnd.Int63n(max-min) + min)
}

// Determines if the cluster is currently upgrading or error if unable to determine
func upgradeInProgress(uc *upgradev1alpha1.UpgradeConfig, cvClient cv.ClusterVersion) (bool, error) {
	// First check all the UpgradeConfigs
	phase := getCurrentUpgradeConfigPhase(uc)
	if phase == upgradev1alpha1.UpgradePhaseUpgrading {
		return true, nil
	}

	// Then check CVO
	version, err := cvClient.GetClusterVersion()
	if err != nil {
		return false, fmt.Errorf("can't determine cluster version")
	}
	for _, condition := range version.Status.Conditions {
		if condition.Type == v1.OperatorProgressing && condition.Status == v1.ConditionTrue {
			return true, nil
		}
	}

	return false, nil
}

// Returns the upgrade phase of the current desired upgrade from the UpgradeConfig
func getCurrentUpgradeConfigPhase(uc *upgradev1alpha1.UpgradeConfig) upgradev1alpha1.UpgradePhase {
	var history upgradev1alpha1.UpgradeHistory
	found := false
	for _, h := range uc.Status.History {
		if h.Version == uc.Spec.Desired.Version {
			history = h
			found = true
		}
	}
	if !found {
		return upgradev1alpha1.UpgradePhaseUnknown
	}
	return history.Phase
}

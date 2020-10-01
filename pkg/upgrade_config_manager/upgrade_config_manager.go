package upgrade_config_manager

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/errors"
	"math"
	"math/rand"
	"reflect"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/policyprovider"
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
	ErrClusterIsUpgrading = fmt.Errorf("cluster is upgrading")
	ErrRetrievingUpgradeConfigs = fmt.Errorf("unable to retrieve upgradeconfigs")
	ErrMissingOperatorNamespace = fmt.Errorf("can't determine operator namespace, missing env OPERATOR_NAMESPACE")
	ErrProviderSpecPull = fmt.Errorf("unable to retrieve upgrade policySpecs")
	ErrRemovingUpgradeConfig = fmt.Errorf("unable to remove existing UpgradeConfig")
	ErrCreatingUpgradeConfig = fmt.Errorf("unable to create new UpgradeConfig")
)

//go:generate mockgen -destination=mocks/upgrade_config_manager.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/upgrade_config_manager UpgradeConfigManager
type UpgradeConfigManager interface {
	Get() (*upgradev1alpha1.UpgradeConfigList, error)
	StartSync(stopCh <-chan struct{})
	Refresh() (bool, error)
}

//go:generate mockgen -destination=mocks/upgrade_config_manager_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/upgrade_config_manager UpgradeConfigManagerBuilder
type UpgradeConfigManagerBuilder interface {
	NewManager(client.Client) (UpgradeConfigManager, error)
}

func NewBuilder() UpgradeConfigManagerBuilder {
	return &upgradeConfigManagerBuilder{}
}

type upgradeConfigManagerBuilder struct{}

type upgradeConfigManager struct {
	client                client.Client
	cvClientBuilder       cv.ClusterVersionBuilder
	policyProviderBuilder policyprovider.PolicyProviderBuilder
	configManagerBuilder  configmanager.ConfigManagerBuilder
}

func (ucb *upgradeConfigManagerBuilder) NewManager(client client.Client) (UpgradeConfigManager, error) {

	ppBuilder := policyprovider.NewBuilder()
	cvBuilder := cv.NewBuilder()
	cmBuilder := configmanager.NewBuilder()

	return &upgradeConfigManager{
		client:                client,
		cvClientBuilder:       cvBuilder,
		policyProviderBuilder: ppBuilder,
		configManagerBuilder:  cmBuilder,
	}, nil
}

func (s *upgradeConfigManager) Get() (*upgradev1alpha1.UpgradeConfigList, error) {
	upgradeConfigs := &upgradev1alpha1.UpgradeConfigList{}
	err := s.client.List(context.TODO(), upgradeConfigs, &client.ListOptions{})
	if err != nil {
		return nil, ErrRetrievingUpgradeConfigs
	}
	return upgradeConfigs, nil
}

// Syncs UpgradeConfigs from the policy provider periodically until the operator is killed or a message is sent on the stopCh
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

// Refreshes UpgradeConfigs from the UpgradeConfig providerApplies the supplied Upgrade Policy to the cluster in the form of an UpgradeConfig
// Returns an indication of if the policy being applied differs to the existing UpgradeConfig,
// and indication of error if one occurs.
func (s *upgradeConfigManager) Refresh() (bool, error) {

	// Get the running namespace
	operatorNS, err := util.GetOperatorNamespace()
	if err != nil {
		return false, ErrMissingOperatorNamespace
	}

	// Get the current UpgradeConfigs on the cluster
	currentUpgradeConfigs, err := s.Get()
	if err != nil {
		return false, ErrRetrievingUpgradeConfigs
	}

	// If we are in the middle of an upgrade, we should not refresh
	cvClient := s.cvClientBuilder.New(s.client)
	upgrading, err := upgradeInProgress(currentUpgradeConfigs, cvClient)
	if err != nil {
		return false, err
	}
	if upgrading {
		return false, ErrClusterIsUpgrading
	}

	// Get the latest config specs from the provider
	pp, err := s.policyProviderBuilder.New(s.client, s.configManagerBuilder)
	if err != nil {
		return false, fmt.Errorf("unable to create policy provider: %v", err)
	}
	policySpecs, err := pp.Get()
	if err != nil {
		log.Error(err, "error pulling provider specs")
		return false, ErrProviderSpecPull
	}

	// If there are no policySpecs, remove any existing UpgradeConfigs
	if len(policySpecs) == 0 {
		if len(currentUpgradeConfigs.Items) > 0 {
			for _, upgradeConfig := range currentUpgradeConfigs.Items {
				log.Info(fmt.Sprintf("Removing expired UpgradeConfig %s", upgradeConfig.Name))
				err = s.client.Delete(context.TODO(), &upgradeConfig)
				if err != nil {
					log.Error(err, "can't remove UpgradeConfig")
					return false, ErrRemovingUpgradeConfig
				}
			}
		}
		return true, nil
	}

	// We are basing on an assumption of one (1) UpgradeConfig per cluster right now.
	// So just use the first policy returned
	if len(policySpecs) > 1 {
		log.Info("More than one Upgrade Policy received, only considering the first.")
	}
	upgradeConfigSpec := policySpecs[0]

	// Set up the UpgradeConfig we will replace with
	replacementUpgradeConfig := upgradev1alpha1.UpgradeConfig{}

	// Check if we have an existing UpgradeConfig to compare against, for the refresh
	originalUpgradeConfig := upgradev1alpha1.UpgradeConfig{}
	if len(currentUpgradeConfigs.Items) > 0 {
		originalUpgradeConfig = currentUpgradeConfigs.Items[0]
		// If there was an existing UpgradeConfig, make a clone of its contents
		originalUpgradeConfig.DeepCopyInto(&replacementUpgradeConfig)
	} else {
		// No existing UpgradeConfig exists, give the new one the default name/namespace
		replacementUpgradeConfig.Name = UPGRADECONFIG_CR_NAME
		replacementUpgradeConfig.Namespace = operatorNS
	}

	// Replace the spec with the refreshed policy spec
	upgradeConfigSpec.DeepCopyInto(&replacementUpgradeConfig.Spec)

	// is there a difference between the original and replacement?
	changed := !reflect.DeepEqual(replacementUpgradeConfig.Spec, originalUpgradeConfig.Spec)
	if changed {
		// Apply the resource
		log.Info("cluster upgrade policy has changed, will update")
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
		log.Info(fmt.Sprintf("no change in policy from existing UpgradeConfig %v, won't update", originalUpgradeConfig.Name))
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
func upgradeInProgress(ucl *upgradev1alpha1.UpgradeConfigList, cvClient cv.ClusterVersion) (bool, error) {
	// First check all the UpgradeConfigs
	for _, uc := range ucl.Items {
		phase := getCurrentUpgradeConfigPhase(uc)
		if phase == upgradev1alpha1.UpgradePhaseUpgrading {
			return true, nil
		}
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
func getCurrentUpgradeConfigPhase(uc upgradev1alpha1.UpgradeConfig) upgradev1alpha1.UpgradePhase {
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

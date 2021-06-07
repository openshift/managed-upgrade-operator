package upgradeconfigmanager

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"time"

	"github.com/jpillora/backoff"

	v1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/specprovider"
	"github.com/openshift/managed-upgrade-operator/util"
)

// ConfigManagerSource is a type illustrating a config manager source
type ConfigManagerSource string

var log = logf.Log.WithName("upgrade-config-manager")

const (
	// UPGRADECONFIG_CR_NAME is the name of the Custom Resource that the provider will manage
	UPGRADECONFIG_CR_NAME = "managed-upgrade-config"
	// JITTER_FACTOR is a jitter factor (percentage / 100) used to alter watch interval
	JITTER_FACTOR = 0.1
	// INITIAL_SYNC_DURATION is an initial sync duration
	INITIAL_SYNC_DURATION = 1 * time.Minute
	// ERROR_RETRY_DURATION is an error retryn duration
	ERROR_RETRY_DURATION = 5 * time.Minute
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
	ErrNotConfigured            = fmt.Errorf("no upgrade config manager configured")
)

// UpgradeConfigManager enables an implementation of an UpgradeConfigManager
//go:generate mockgen -destination=mocks/upgradeconfigmanager.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager UpgradeConfigManager
type UpgradeConfigManager interface {
	Get() (*upgradev1alpha1.UpgradeConfig, error)
	StartSync(stopCh <-chan struct{})
	Refresh() (bool, error)
}

// UpgradeConfigManagerBuilder enables an implementation of an UpgradeConfigManagerBuilder
//go:generate mockgen -destination=mocks/upgradeconfigmanager_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager UpgradeConfigManagerBuilder
type UpgradeConfigManagerBuilder interface {
	NewManager(client.Client) (UpgradeConfigManager, error)
}

// NewBuilder returns an upgradeConfigManagerBuilder
func NewBuilder() UpgradeConfigManagerBuilder {
	return &upgradeConfigManagerBuilder{}
}

type upgradeConfigManagerBuilder struct{}

type upgradeConfigManager struct {
	client               client.Client
	cvClientBuilder      cv.ClusterVersionBuilder
	specProviderBuilder  specprovider.SpecProviderBuilder
	configManagerBuilder configmanager.ConfigManagerBuilder
	metricsBuilder       metrics.MetricsBuilder
	backoffCounter       *backoff.Backoff
}

func (ucb *upgradeConfigManagerBuilder) NewManager(client client.Client) (UpgradeConfigManager, error) {

	spBuilder := specprovider.NewBuilder()
	cvBuilder := cv.NewBuilder()
	cmBuilder := configmanager.NewBuilder()
	mBuilder := metrics.NewBuilder()
	b := &backoff.Backoff{
		Min:    1 * time.Minute,
		Max:    1 * time.Hour,
		Factor: 2,
		Jitter: false,
	}

	return &upgradeConfigManager{
		client:               client,
		cvClientBuilder:      cvBuilder,
		specProviderBuilder:  spBuilder,
		configManagerBuilder: cmBuilder,
		metricsBuilder:       mBuilder,
		backoffCounter:       b,
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

	metricsClient, err := s.metricsBuilder.NewClient(s.client)
	if err != nil {
		log.Error(err, "can't create metrics client")
		return
	}

	duration := durationWithJitter(INITIAL_SYNC_DURATION, JITTER_FACTOR)
	for {
		select {
		case <-time.After(duration):
			_, err := s.Refresh()
			if err != nil {
				waitDuration := s.backoffCounter.Duration()
				log.Error(err, fmt.Sprintf("unable to refresh upgrade config, retrying in %v", waitDuration))
				metricsClient.UpdateMetricUpgradeConfigSynced(UPGRADECONFIG_CR_NAME)
				duration = durationWithJitter(waitDuration, JITTER_FACTOR)
			} else {
				s.backoffCounter.Reset()
				metricsClient.ResetMetricUpgradeConfigSynced(UPGRADECONFIG_CR_NAME)
				duration = durationWithJitter(cfg.GetWatchInterval(), JITTER_FACTOR)
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
		log.Info("skipping spec refresh as the cluster is currently upgrading")
		return false, nil
	}

	// Get the latest config specs from the provider
	pp, err := s.specProviderBuilder.New(s.client, s.configManagerBuilder)
	if err != nil {
		// If the spec provider config doesn't exist, return indicatively that no UC Mgr is configured
		if err == specprovider.ErrNoSpecProviderConfig {
			return false, ErrNotConfigured
		}
		return false, err
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
				log.Error(err, "can't remove UpgradeConfig after finding no upgrade_policy")
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
		if foundUpgradeConfig {
			// Delete and re-create the resource
			log.Info("cluster upgrade spec has changed, will delete and re-create.")
			confirmDeletedUpgrade := false
			gracePeriod := int64(0)
			deleteOptions := &client.DeleteOptions{
				GracePeriodSeconds: &gracePeriod,
			}

			err = s.client.Delete(context.TODO(), currentUpgradeConfig, deleteOptions)
			if err != nil {
				if err == ErrUpgradeConfigNotFound {
					log.Info("UpgradeConfig already deleted")
					confirmDeletedUpgrade = true
				} else {
					log.Error(err, "can't remove UpgradeConfig during re-create")
					return false, ErrRemovingUpgradeConfig
				}
			}

			// Confirm object is deleted prior to creating
			if !confirmDeletedUpgrade {
				err = wait.PollImmediate(5*time.Second, 60*time.Second, func() (bool, error) {
					_, err := s.Get()
					if err != nil {
						if err == ErrUpgradeConfigNotFound {
							log.Info("UpgradeConfig deletion confirmed")
							return true, nil
						}
						return false, err
					}
					return false, nil
				})
				if err != nil {
					return false, fmt.Errorf("Unable to confirm deletion of current UpgradeConfig: %v", err)
				}
			}
		}

		replacementUpgradeConfig.SetResourceVersion("")

		err = s.client.Create(context.TODO(), &replacementUpgradeConfig)
		if err != nil {
			return false, fmt.Errorf("unable to apply UpgradeConfig changes: %v", err)
		}
		log.Info("Successfully create new UpgradeConfig")
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

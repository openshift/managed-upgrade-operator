package ocmmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"time"

	"github.com/blang/semver"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/util"
)

const (
	// Header field used to correlate OCM events
	OPERATION_ID_HEADER = "X-Operation-Id"
	// Path to the OCM clusters service
	CLUSTERS_V1_PATH = "/api/clusters_mgmt/v1/clusters"
	// Sub-path to the OCM upgrade policies service
	UPGRADEPOLICIES_V1_PATH = "upgrade_policies"
	// Name of the Custom Resource that the provider will manage
	UPGRADECONFIG_CR_NAME = "osd-upgrade-config"
	// Jitter factor (percentage / 100) used to alter watch interval
	JITTER_FACTOR = 0.1
)

var log = logf.Log.WithName("upgrade-config-manager")

func NewManager(client client.Client) (*osdUpgradeConfigManager, error) {

	log.Info("Initializing the upgradeConfigManager")

	// Fetch the cluster AccessToken
	accessToken, err := util.GetAccessToken(client)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve cluster access token")
	}
	// Set up the HTTP client using the token
	httpClient := &http.Client{
		Transport: &ocmRoundTripper{
			authorization: *accessToken,
		},
	}

	cfg, err := readConfigManagerConfig(client)
	if err != nil {
		return nil, fmt.Errorf("failed to read OCM configManager configuration: %v", err)
	}

	return &osdUpgradeConfigManager{
		client:     client,
		config:     cfg,
		httpClient: httpClient,
	}, nil
}

type osdUpgradeConfigManager struct {
	// Cluster k8s client
	client client.Client
	// for retrieving config manager configuration
	config *ocmUpgradeConfigManagerConfig
	// HTTP client used for API queries (TODO: remove in favour of OCM SDK)
	httpClient *http.Client
}

// Represents an unmarshalled Upgrade Policy response from Cluster Services
type upgradePolicyList struct {
	Kind  string          `json:"kind"`
	Page  int64           `json:"page"`
	Size  int64           `json:"size"`
	Total int64           `json:"total"`
	Items []upgradePolicy `json:"items"`
}

// Represents an unmarshalled individual Upgrade Policy response from Cluster Services
type upgradePolicy struct {
	Id                   string               `json:"id"`
	Kind                 string               `json:"kind"`
	Href                 string               `json:"href"`
	Schedule             string               `json:"schedule"`
	ScheduleType         string               `json:"schedule_type"`
	UpgradeType          string               `json:"upgrade_type"`
	Version              string               `json:"version"`
	NextRun              string               `json:"next_run"`
	PrevRun              string               `json:"prev_run"`
	NodeDrainGracePeriod nodeDrainGracePeriod `json:"node_drain_grace_period"`
	ClusterId            string               `json:"cluster_id"`
}

type nodeDrainGracePeriod struct {
	Value int64  `json:"value"`
	Unit  string `json:"unit"`
}

// Represents an unmarshalled Cluster List response from Cluster Services
type clusterList struct {
	Kind  string        `json:"kind"`
	Page  int64         `json:"page"`
	Size  int64         `json:"size"`
	Total int64         `json:"total"`
	Items []clusterInfo `json:"items"`
}

// Represents a partial unmarshalled Cluster response from Cluster Services
type clusterInfo struct {
	Id      string         `json:"id"`
	Version clusterVersion `json:"version"`
}

type clusterVersion struct {
	Id           string `json:"id"`
	ChannelGroup string `json:"channel_group"`
}

type ocmRoundTripper struct {
	authorization util.AccessToken
}

func (ort *ocmRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	authVal := fmt.Sprintf("AccessToken %s:%s", ort.authorization.ClusterId, ort.authorization.PullSecret)
	req.Header.Add("Authorization", authVal)
	transport := http.Transport{
		TLSHandshakeTimeout: time.Second * 5,
	}
	return transport.RoundTrip(req)
}

// UpgradeConfigManager will trigger X every `WatchIntervalMinutes` and only stop if the operator is killed or a
// message is sent on the stopCh
func (s *osdUpgradeConfigManager) Start(stopCh <-chan struct{}) {
	log.Info("Starting the upgradeConfigManager")

	initialSync := true
	for {

		// Select a new watch interval with jitter
		duration := durationWithJitter(s.config.GetWatchInterval(), JITTER_FACTOR)

		// Don't wait if this is the first run since starting
		if initialSync {
			duration = 0
			initialSync = false
		}

		select {
		case <-time.After(duration):
			_, err := s.RefreshUpgradeConfig()
			if err != nil {
				log.Error(err, "unable to refresh upgrade config")
			}
		case <-stopCh:
			log.Info("Stopping the upgradeConfigManager")
			break
		}
	}
}

func readConfigManagerConfig(client client.Client) (*ocmUpgradeConfigManagerConfig, error) {
	ns, err := getOperatorNamespace()
	if err != nil {
		return nil, err
	}

	cfb := configmanager.NewBuilder()
	cfm := cfb.New(client, ns)
	cfg := &ocmUpgradeConfigManagerConfig{}
	err = cfm.Into(cfg)
	if err != nil {
		return nil, err
	}

	err = cfg.IsValid()
	return cfg, err
}

// The primary function called each watch interval to synchronise the cluster's
// UpgradeConfig against the Cluster Service's Upgrade Policy.
// Returns an indication of if the upgrade config had changed during the refresh,
// and indication of error if one occurs.
func (s *osdUpgradeConfigManager) RefreshUpgradeConfig() (bool, error) {

	cluster, err := getClusterFromOCMApi(s.client, s.httpClient, s.config)
	if err != nil {
		log.Error(err, "cannot obtain internal cluster ID")
		return false, err
	}
	if cluster.Id == "" {
		return false, fmt.Errorf("cluster ID not found via OCM")
	}
	if cluster.Version.ChannelGroup == "" {
		return false, fmt.Errorf("cluster channel group not set or found via OCM")
	}

	// Retrieve the cluster's upgrade policy from Cluster Services
	upgradePolicies, err := getClusterUpgradePolicies(cluster, s.httpClient, s.config)
	if err != nil {
		log.Error(err, "cannot fetch next upgrade status")
		return false, err
	}

	// Apply the Upgrade Policies to the cluster
	changed, err := applyUpgradePolicies(upgradePolicies, cluster, s.client)
	if err != nil {
		log.Error(err, "cannot apply upgrade config")
		return false, err
	}

	return changed, nil
}

// Queries and returns the Upgrade Policy from Cluster Services
func getClusterUpgradePolicies(cluster *clusterInfo, client *http.Client, cfg *ocmUpgradeConfigManagerConfig) (*upgradePolicyList, error) {

	url, err := cfg.GetOCMBaseURL()
	if err != nil {
		return nil, fmt.Errorf("can't read OCM API url: %v", err)
	}
	url.Path = path.Join(url.Path, CLUSTERS_V1_PATH, cluster.Id, UPGRADEPOLICIES_V1_PATH)

	request := &http.Request{
		Method: "GET",
		URL:    url,
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("can't query upgrade service: %v", err)
	}
	operationId := response.Header[OPERATION_ID_HEADER]

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received error code '%v' from OCM upgrade policy service, operation id '%v'", response.StatusCode, operationId)
	}

	if response.Body != nil {
		defer response.Body.Close()
	}

	var upgradeResponse upgradePolicyList
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&upgradeResponse)
	if err != nil {
		log.Error(err, "unable to decode OCM upgrade policy response, operation id '%v'", operationId)
		return nil, err
	}

	return &upgradeResponse, nil
}

// Applies the supplied Upgrade Policy to the cluster in the form of an UpgradeConfig
// Returns an indication of if the policy being applied differs to the existing UpgradeConfig,
// and indication of error if one occurs.
func applyUpgradePolicies(upgradePolicies *upgradePolicyList, cluster *clusterInfo, c client.Client) (bool, error) {
	// Fetch the current UpgradeConfig instance, if it exists
	upgradeConfigs := &upgradev1alpha1.UpgradeConfigList{}
	err := c.List(context.TODO(), upgradeConfigs, &client.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("unable to retrieve upgradeconfigs")
	}

	// If there are no policies available, or if there is no next upgrade scheduled, do not retain any UpgradeConfigs
	if len(upgradePolicies.Items) == 0 || upgradePolicies.Items[0].NextRun == "" {
		if len(upgradeConfigs.Items) > 0 {
			for _, upgradeConfig := range upgradeConfigs.Items {
				log.Info(fmt.Sprintf("Removing expired UpgradeConfig %s", upgradeConfig.Name))
				err = c.Delete(context.TODO(), &upgradeConfig)
				if err != nil {
					return false, fmt.Errorf("unable to delete existing upgrade config: %v", err)
				}
			}
		}
		return true, nil
	}

	// Only handling singular upgrade policies for now - but log if that arises
	if len(upgradePolicies.Items) > 1 {
		log.Info("More than one Upgrade Policy received, only considering the first.")
	}
	upgradePolicy := upgradePolicies.Items[0]

	// Set up an UpgradeConfig that reflects the policy
	replacementUpgradeConfig := upgradev1alpha1.UpgradeConfig{}

	// Check if we have an existing UpgradeConfig to compare against, for the refresh
	originalUpgradeConfig := upgradev1alpha1.UpgradeConfig{}
	if len(upgradeConfigs.Items) > 0 {
		originalUpgradeConfig = upgradeConfigs.Items[0]
		// If there was an existing UpgradeConfig, make a clone of its contents
		originalUpgradeConfig.DeepCopyInto(&replacementUpgradeConfig)
	} else {
		// No existing UpgradeConfig exists, give the new one the default name/namespace
		operatorNS, err := getOperatorNamespace()
		if err != nil {
			return false, fmt.Errorf("unable to determine running namespace, missing env OPERATOR_NAMESPACE")
		}
		replacementUpgradeConfig.Name = UPGRADECONFIG_CR_NAME
		replacementUpgradeConfig.Namespace = operatorNS
	}

	// And build up the replacement UpgradeConfig based on the policy
	// determine channel from channel group
	upgradeChannel, err := inferUpgradeChannelFromChannelGroup(cluster.Version.ChannelGroup, upgradePolicy.Version)
	if err != nil {
		return false, fmt.Errorf("unable to determine channel from channel group '%v' and version '%v'", cluster.Version.ChannelGroup, upgradePolicy.Version)
	}
	replacementUpgradeConfig.Spec = upgradev1alpha1.UpgradeConfigSpec{
		Desired: upgradev1alpha1.Update{
			Version: upgradePolicy.Version,
			Channel: *upgradeChannel,
		},
		UpgradeAt:            upgradePolicy.NextRun,
		PDBForceDrainTimeout: int32(upgradePolicy.NodeDrainGracePeriod.Value),
		Type:                 upgradev1alpha1.UpgradeType(upgradePolicy.UpgradeType),
	}
	replacementUpgradeConfig.Status = upgradev1alpha1.UpgradeConfigStatus{}

	// is there a difference between the original and replacement?
	changed := !reflect.DeepEqual(replacementUpgradeConfig.Spec, originalUpgradeConfig.Spec)
	if changed {
		// Apply the resource
		log.Info("cluster upgrade policy has changed, will update")
		err = c.Update(context.TODO(), &replacementUpgradeConfig)
		if err != nil {
			if errors.IsNotFound(err) {
				// couldn't update because it didn't exist - create it instead.
				err = c.Create(context.TODO(), &replacementUpgradeConfig)
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

// getOperatorNamespace retrieves the operators namespace from an environment variable and returns it to the caller.
func getOperatorNamespace() (string, error) {
	envVarOperatorNamespace := "OPERATOR_NAMESPACE"
	ns, found := os.LookupEnv(envVarOperatorNamespace)
	if !found {
		return "", fmt.Errorf("%s must be set", envVarOperatorNamespace)
	}
	return ns, nil
}

// Infers a CVO channel name from the channel group and TO desired version edges
func inferUpgradeChannelFromChannelGroup(channelGroup string, toVersion string) (*string, error) {

	toSV, err := semver.Parse(toVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid semantic TO version: %v", toVersion)
	}

	channel := fmt.Sprintf("%v-%v.%v", channelGroup, toSV.Major, toSV.Minor)
	return &channel, nil
}

// Read cluster info from OCM
func getClusterFromOCMApi(kc client.Client, client *http.Client, cfg *ocmUpgradeConfigManagerConfig) (*clusterInfo, error) {

	// fetch the clusterversion, which contains the internal ID
	cv := &configv1.ClusterVersion{}
	err := kc.Get(context.TODO(), types.NamespacedName{Name: "version"}, cv)
	if err != nil {
		return nil, fmt.Errorf("can't get clusterversion: %v", err)
	}
	externalID := cv.Spec.ClusterID

	search := fmt.Sprintf("external_id = '%s'", externalID)
	query := make(url.Values)
	query.Add("page", "1")
	query.Add("size", "1")
	query.Add("search", search)

	url, err := url.Parse(cfg.ConfigManagerConfig.OcmBaseURL)
	if err != nil {
		log.Error(err, "OCM API URL unparsable.")
		return nil, fmt.Errorf("can't parse OCM API url: %v", err)
	}
	url.Path = path.Join(url.Path, CLUSTERS_V1_PATH)
	url.RawQuery = query.Encode()
	request := &http.Request{
		Method: "GET",
		URL:    url,
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("can't query OCM cluster service: %v", err)
	}
	operationId := response.Header[OPERATION_ID_HEADER]
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received error code %v, operation id '%v'", response.StatusCode, operationId)
	}

	if response.Body != nil {
		defer response.Body.Close()
	}

	var listResponse clusterList
	decoder := json.NewDecoder(response.Body)
	err = decoder.Decode(&listResponse)
	if err != nil {
		log.Error(err, "unable to decode OCM cluster response, operation id '%v'", operationId)
		return nil, err
	}
	if listResponse.Size != 1 || len(listResponse.Items) != 1 {
		return nil, fmt.Errorf("no items returned from OCM cluster service, operation id '%v'", operationId)
	}

	return &listResponse.Items[0], nil
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

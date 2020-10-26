package ocmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/blang/semver"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/util"
)

const (
	// Header field used to correlate OCM events
	OPERATION_ID_HEADER = "X-Operation-Id"
	// Path to the OCM clusters service
	CLUSTERS_V1_PATH = "/api/clusters_mgmt/v1/clusters"
	// Sub-path to the OCM upgrade policies service
	UPGRADEPOLICIES_V1_PATH = "upgrade_policies"
)

var log = logf.Log.WithName("ocm-config-getter")

// Errors
var (
	ErrProviderUnavailable = fmt.Errorf("OCM Provider unavailable")
	ErrClusterIdNotFound   = fmt.Errorf("cluster ID can't be found")
	ErrMissingChannelGroup = fmt.Errorf("channel group not returned or empty")
	ErrRetrievingPolicies  = fmt.Errorf("could not retrieve provider upgrade policies")
	ErrProcessingPolicies  = fmt.Errorf("could not process provider upgrade policies")
)

func New(client client.Client, ocmBaseUrl *url.URL) (*ocmProvider, error) {

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

	return &ocmProvider{
		client:     client,
		ocmBaseUrl: ocmBaseUrl,
		httpClient: httpClient,
	}, nil
}

type ocmProvider struct {
	// Cluster k8s client
	client client.Client
	// Base OCM API Url
	ocmBaseUrl *url.URL
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

// Returns an indication of if the upgrade config had changed during the refresh,
// and indication of error if one occurs.
func (s *ocmProvider) Get() ([]upgradev1alpha1.UpgradeConfigSpec, error) {

	log.Info("Commencing sync with OCM Spec provider")

	cluster, err := getClusterFromOCMApi(s.client, s.httpClient, s.ocmBaseUrl)
	if err != nil {
		log.Error(err, "cannot obtain internal cluster ID")
		return nil, ErrProviderUnavailable
	}
	if cluster.Id == "" {
		return nil, ErrClusterIdNotFound
	}
	if cluster.Version.ChannelGroup == "" {
		return nil, ErrMissingChannelGroup
	}

	// Retrieve the cluster's available upgrade policies from Cluster Services
	upgradePolicies, err := getClusterUpgradePolicies(cluster, s.httpClient, s.ocmBaseUrl)
	if err != nil {
		log.Error(err, "error retrieving upgrade policies")
		return nil, ErrRetrievingPolicies
	}

	// Get the next occurring policy from the available policies
	if len(upgradePolicies.Items) > 0 {
		nextOccurringUpgradePolicy, err := getNextOccurringUpgradePolicy(upgradePolicies)
		if err != nil {
			log.Error(err, "error getting next upgrade policy from upgrade policies")
			return nil, err
		}
		log.Info(fmt.Sprintf("Detected upgrade policy %s as next occurring.", nextOccurringUpgradePolicy.Id))

		// Apply the next occurring Upgrade policy to the clusters UpgradeConfig CR.
		specs, err := buildUpgradeConfigSpecs(nextOccurringUpgradePolicy, cluster, s.client)
		if err != nil {
			log.Error(err, "cannot build UpgradeConfigs from policy")
			return nil, ErrProcessingPolicies
		}

		return specs, nil
	}
	log.Info("No upgrade policies available")
	return nil, nil
}

// Queries and returns the Upgrade Policy from Cluster Services
func getClusterUpgradePolicies(cluster *clusterInfo, client *http.Client, ocmBaseUrl *url.URL) (*upgradePolicyList, error) {

	upUrl, err := url.Parse(ocmBaseUrl.String())
	if err != nil {
		return nil, fmt.Errorf("can't read OCM API url: %v", err)
	}
	upUrl.Path = path.Join(upUrl.Path, CLUSTERS_V1_PATH, cluster.Id, UPGRADEPOLICIES_V1_PATH)

	request := &http.Request{
		Method: "GET",
		URL:    upUrl,
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

	var upgradeResponses upgradePolicyList
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&upgradeResponses)
	if err != nil {
		return nil, fmt.Errorf("unable to decode OCM upgrade policy response, operation id '%v'", operationId)
	}

	return &upgradeResponses, nil
}

// getNextOccurringUpgradePolicy returns the next occurring upgradepolicy from a list of upgrade
// policies, regardless of the schedule_type.
func getNextOccurringUpgradePolicy(uPs *upgradePolicyList) (*upgradePolicy, error) {
	var nextOccurringUpgradePolicy upgradePolicy

	nextOccurringUpgradePolicy = uPs.Items[0]

	for _, uP := range uPs.Items {
		currentNext, err := time.Parse(time.RFC3339, nextOccurringUpgradePolicy.NextRun)
		if err != nil {
			return &nextOccurringUpgradePolicy, err
		}
		evalNext, err := time.Parse(time.RFC3339, uP.NextRun)
		if err != nil {
			return &nextOccurringUpgradePolicy, err
		}

		if evalNext.Before(currentNext) {
			nextOccurringUpgradePolicy = uP
		}
	}

	return &nextOccurringUpgradePolicy, nil
}

// Applies the supplied Upgrade Policy to the cluster in the form of an UpgradeConfig
// Returns an indication of if the policy being applied differs to the existing UpgradeConfig,
// and indication of error if one occurs.
func buildUpgradeConfigSpecs(upgradePolicy *upgradePolicy, cluster *clusterInfo, c client.Client) ([]upgradev1alpha1.UpgradeConfigSpec, error) {

	upgradeConfigSpecs := make([]upgradev1alpha1.UpgradeConfigSpec, 0)

	upgradeChannel, err := inferUpgradeChannelFromChannelGroup(cluster.Version.ChannelGroup, upgradePolicy.Version)
	if err != nil {
		return nil, fmt.Errorf("unable to determine channel from channel group '%v' and version '%v' for policy ID '%v'", cluster.Version.ChannelGroup, upgradePolicy.Version, upgradePolicy.Id)
	}
	upgradeConfigSpec := upgradev1alpha1.UpgradeConfigSpec{
		Desired: upgradev1alpha1.Update{
			Version: upgradePolicy.Version,
			Channel: *upgradeChannel,
		},
		UpgradeAt:            upgradePolicy.NextRun,
		PDBForceDrainTimeout: int32(upgradePolicy.NodeDrainGracePeriod.Value),
		Type:                 upgradev1alpha1.UpgradeType(upgradePolicy.UpgradeType),
	}
	upgradeConfigSpecs = append(upgradeConfigSpecs, upgradeConfigSpec)

	return upgradeConfigSpecs, nil
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
func getClusterFromOCMApi(kc client.Client, client *http.Client, ocmApi *url.URL) (*clusterInfo, error) {

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

	csUrl, err := url.Parse(ocmApi.String())
	if err != nil {
		return nil, fmt.Errorf("can't parse OCM API url: %v", err)
	}
	csUrl.Path = path.Join(csUrl.Path, CLUSTERS_V1_PATH)
	csUrl.RawQuery = query.Encode()
	request := &http.Request{
		Method: "GET",
		URL:    csUrl,
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
		return nil, fmt.Errorf("unable to decode OCM cluster response, operation id '%v'", operationId)
	}
	if listResponse.Size != 1 || len(listResponse.Items) != 1 {
		return nil, fmt.Errorf("no items returned from OCM cluster service, operation id '%v'", operationId)
	}

	return &listResponse.Items[0], nil
}

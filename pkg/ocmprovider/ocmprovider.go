package ocmprovider

import (
	"context"
	"fmt"
	"github.com/go-resty/resty/v2"
	"net/http"
	"net/url"
	"path"
	"strings"
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
	// Sub-path to the policy state service
	STATE_V1_PATH = "state"
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
	httpClient := resty.New().SetTransport(&ocmRoundTripper{authorization: *accessToken})

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
	httpClient *resty.Client
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
	Id           string `json:"id"`
	Kind         string `json:"kind"`
	Href         string `json:"href"`
	Schedule     string `json:"schedule"`
	ScheduleType string `json:"schedule_type"`
	UpgradeType  string `json:"upgrade_type"`
	Version      string `json:"version"`
	NextRun      string `json:"next_run"`
	PrevRun      string `json:"prev_run"`
	ClusterId    string `json:"cluster_id"`
}

// Represents an Upgrade Policy state for notifications
type upgradePolicyState struct {
	Kind        string `json:"kind"`
	Href        string `json:"href"`
	Value       string `json:"value"`
	Description string `json:"description"`
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
	Id                   string               `json:"id"`
	Version              clusterVersion       `json:"version"`
	NodeDrainGracePeriod nodeDrainGracePeriod `json:"node_drain_grace_period"`
}

type nodeDrainGracePeriod struct {
	Value int64  `json:"value"`
	Unit  string `json:"unit"`
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
		// Pass the error up the chain if the cluster ID couldn't be found
		if err == ErrClusterIdNotFound {
			return nil, err
		}
		return nil, ErrProviderUnavailable
	}
	// In case a response was returned that has no cluster ID
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

		policyState, err := getUpgradePolicyState(cluster, nextOccurringUpgradePolicy, s.httpClient, s.ocmBaseUrl)
		if err != nil {
			log.Error(err, "error getting policy's state")
			return nil, err
		}

		if !isActionableUpgradePolicy(nextOccurringUpgradePolicy, policyState) {
			return nil, nil
		}

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

func getUpgradePolicyState(cluster *clusterInfo, up *upgradePolicy, client *resty.Client, ocmBaseUrl *url.URL) (*upgradePolicyState, error) {

	upUrl, err := url.Parse(ocmBaseUrl.String())
	if err != nil {
		return nil, fmt.Errorf("can't read OCM API url: %v", err)
	}
	upUrl.Path = path.Join(upUrl.Path, CLUSTERS_V1_PATH, cluster.Id, UPGRADEPOLICIES_V1_PATH, up.Id, STATE_V1_PATH)

	response, err := client.R().
		SetResult(&upgradePolicyState{}).
		ExpectContentType("application/json").
		Get(upUrl.String())
	if err != nil {
		return nil, fmt.Errorf("can't send notification: %v", err)
	}
	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return nil, fmt.Errorf("received error code '%v' from OCM upgrade policy service, operation id '%v'", response.StatusCode(), operationId)
	}

	stateResponse := response.Result().(*upgradePolicyState)
	return stateResponse, nil
}

// Queries and returns the Upgrade Policy from Cluster Services
func getClusterUpgradePolicies(cluster *clusterInfo, client *resty.Client, ocmBaseUrl *url.URL) (*upgradePolicyList, error) {

	upUrl, err := url.Parse(ocmBaseUrl.String())
	if err != nil {
		return nil, fmt.Errorf("can't read OCM API url: %v", err)
	}
	upUrl.Path = path.Join(upUrl.Path, CLUSTERS_V1_PATH, cluster.Id, UPGRADEPOLICIES_V1_PATH)

	response, err := client.R().
		SetResult(&upgradePolicyList{}).
		ExpectContentType("application/json").
		Get(upUrl.String())

	if err != nil {
		return nil, fmt.Errorf("can't query upgrade service: %v", err)
	}
	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return nil, fmt.Errorf("received error code %v, operation id '%v'", response.StatusCode(), operationId)
	}

	upgradeResponses := response.Result().(*upgradePolicyList)

	return upgradeResponses, nil
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

// Checks if the supplied upgrade policy is one which warrants turning into an
// UpgradeConfig
func isActionableUpgradePolicy(up *upgradePolicy, state *upgradePolicyState) bool {

	// Policies that aren't in a SCHEDULED state should be ignored
	if strings.ToLower(state.Value) != "scheduled" {
		return false
	}

	// Automatic upgrade policies will have an empty version if the cluster is up to date
	if len(up.Version) == 0 {
		log.Info(fmt.Sprintf("Upgrade policy %v has an empty version, will ignore.", up.Id))
		return false
	}

	return true
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
		PDBForceDrainTimeout: int32(cluster.NodeDrainGracePeriod.Value),
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
func getClusterFromOCMApi(kc client.Client, client *resty.Client, ocmApi *url.URL) (*clusterInfo, error) {

	// fetch the clusterversion, which contains the internal ID
	cv := &configv1.ClusterVersion{}
	err := kc.Get(context.TODO(), types.NamespacedName{Name: "version"}, cv)
	if err != nil {
		return nil, fmt.Errorf("can't get clusterversion: %v", err)
	}
	externalID := cv.Spec.ClusterID

	csUrl, err := url.Parse(ocmApi.String())
	if err != nil {
		return nil, fmt.Errorf("can't parse OCM API url: %v", err)
	}
	csUrl.Path = path.Join(csUrl.Path, CLUSTERS_V1_PATH)

	response, err := client.R().
		SetQueryParams(map[string]string{
			"page":   "1",
			"size":   "1",
			"search": fmt.Sprintf("external_id = '%s'", externalID),
		}).
		SetResult(&clusterList{}).
		ExpectContentType("application/json").
		Get(csUrl.String())

	if err != nil {
		return nil, fmt.Errorf("can't query OCM cluster service: %v", err)
	}

	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return nil, fmt.Errorf("received error code %v, operation id '%v'", response.StatusCode(), operationId)
	}

	listResponse := response.Result().(*clusterList)
	if listResponse.Size != 1 || len(listResponse.Items) != 1 {
		return nil, ErrClusterIdNotFound
	}

	return &listResponse.Items[0], nil
}

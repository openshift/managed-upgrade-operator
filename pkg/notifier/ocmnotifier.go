package notifier

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/go-resty/resty/v2"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
	"github.com/openshift/managed-upgrade-operator/util"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func NewOCMNotifier(client client.Client, ocmBaseUrl *url.URL, upgradeConfigManager upgradeconfigmanager.UpgradeConfigManager) (*ocmNotifier, error) {

	// Fetch the cluster AccessToken
	accessToken, err := util.GetAccessToken(client)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve cluster access token")
	}

	// Set up the HTTP client using the token
	httpClient := resty.New().SetTransport(&ocmRoundTripper{authorization: *accessToken})

	return &ocmNotifier{
		client:               client,
		ocmBaseUrl:           ocmBaseUrl,
		httpClient:           httpClient,
		upgradeConfigManager: upgradeConfigManager,
	}, nil
}

type ocmNotifier struct {
	// Cluster k8s client
	client client.Client
	// Base OCM API Url
	ocmBaseUrl *url.URL
	// HTTP client used for API queries (TODO: remove in favour of OCM SDK)
	httpClient *resty.Client
	// Retrieves the upgrade config from the cluster
	upgradeConfigManager upgradeconfigmanager.UpgradeConfigManager
}

func (s *ocmNotifier) NotifyState(value NotifyState, description string) error {

	clusterId, err := s.getInternalClusterId()
	if err != nil {
		return err
	}
	policyId, err := s.getPolicyIdForUpgradeConfig(*clusterId)
	if err != nil {
		return fmt.Errorf("can't determine policy ID to notify for: %v", err)
	}

	currentState, err := s.getClusterUpgradePolicyState(*policyId, *clusterId)
	if err != nil {
		return fmt.Errorf("can't determine policy state: %v", err)
	}

	// Don't notify if the state is already at the same value
	if currentState.Value == string(value) {
		return nil
	}

	err = s.sendNotification(value, description, *policyId, *clusterId)
	if err != nil {
		return fmt.Errorf("can't send notification: %v", err)
	}
	return nil
}

// Determines the Cluster Services Upgrade Policy ID corresponding to the UpgradeConfig
func (s *ocmNotifier) getPolicyIdForUpgradeConfig(clusterId string) (*string, error) {
	// Get current UC
	uc, err := s.upgradeConfigManager.Get()
	if err != nil {
		return nil, err
	}

	// Get current policies
	policies, err := s.getClusterUpgradePolicies(clusterId)
	if err != nil {
		return nil, err
	}

	// Find the policy that matches our UC
	foundPolicy := false
	policyId := ""
	for _, policy := range policies.Items {
		if policy.Version == uc.Spec.Desired.Version && policy.NextRun == uc.Spec.UpgradeAt {
			foundPolicy = true
			policyId = policy.Id
		}
	}

	if !foundPolicy {
		return nil, fmt.Errorf("no policy matches the current UpgradeConfig")
	}

	return &policyId, nil
}

// Send a notification of state
func (s *ocmNotifier) sendNotification(value NotifyState, description string, policyId string, clusterId string) error {

	policyState := upgradePolicyStateRequest{
		Value:       string(value),
		Description: description,
	}

	// Create the URL path to send to
	reqUrl, err := url.Parse(s.ocmBaseUrl.String())
	if err != nil {
		return fmt.Errorf("can't read OCM API url: %v", err)
	}
	reqUrl.Path = path.Join(reqUrl.Path, CLUSTERS_V1_PATH, clusterId, UPGRADEPOLICIES_V1_PATH, policyId, STATE_V1_PATH)

	response, err := s.httpClient.R().
		SetHeader("Content-Type", "application/json").
		SetBody(policyState).
		ExpectContentType("application/json").
		Patch(reqUrl.String())

	if err != nil {
		return fmt.Errorf("can't send notification: %v", err)
	}
	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return fmt.Errorf("received error code %v, operation id '%v'", response.StatusCode(), operationId)
	}

	return nil
}

// Queries and returns the Upgrade Policy from Cluster Services
func (s *ocmNotifier) getClusterUpgradePolicies(clusterId string) (*upgradePolicyList, error) {

	upUrl, err := url.Parse(s.ocmBaseUrl.String())
	if err != nil {
		return nil, fmt.Errorf("can't read OCM API url: %v", err)
	}
	upUrl.Path = path.Join(upUrl.Path, CLUSTERS_V1_PATH, clusterId, UPGRADEPOLICIES_V1_PATH)

	response, err := s.httpClient.R().
		SetResult(&upgradePolicyList{}).
		ExpectContentType("application/json").
		Get(upUrl.String())

	if err != nil {
		return nil, fmt.Errorf("can't send notification: %v", err)
	}
	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return nil, fmt.Errorf("received error code '%v' from OCM upgrade policy service, operation id '%v'", response.StatusCode(), operationId)
	}

	upgradeResponse := response.Result().(*upgradePolicyList)
	return upgradeResponse, nil
}

// Queries and returns the Upgrade Policy state from Cluster Services
func (s *ocmNotifier) getClusterUpgradePolicyState(policyId string, clusterId string) (*upgradePolicyState, error) {

	upUrl, err := url.Parse(s.ocmBaseUrl.String())
	if err != nil {
		return nil, fmt.Errorf("can't read OCM API url: %v", err)
	}
	upUrl.Path = path.Join(upUrl.Path, CLUSTERS_V1_PATH, clusterId, UPGRADEPOLICIES_V1_PATH, policyId, STATE_V1_PATH)

	response, err := s.httpClient.R().
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
		return nil, fmt.Errorf("no items returned from OCM cluster service, operation id '%v'", operationId)
	}

	return &listResponse.Items[0], nil

}

func (s *ocmNotifier) getInternalClusterId() (*string, error) {
	cluster, err := getClusterFromOCMApi(s.client, s.httpClient, s.ocmBaseUrl)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve internal ocm cluster ID: %v", err)
	}
	return &cluster.Id, nil
}

// Everything below should eventually be replaced with OCM SDK calls

// Represents an Upgrade Policy state for notifications
type upgradePolicyStateRequest struct {
	Value       string `json:"value"`
	Description string `json:"description"`
}

// Represents an Upgrade Policy state for notifications
type upgradePolicyState struct {
	Kind        string `json:"kind"`
	Href        string `json:"href"`
	Value       string `json:"value"`
	Description string `json:"description"`
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

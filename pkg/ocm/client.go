package ocm

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/go-resty/resty/v2"
	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/util"
)

const (
	// OPERATION_ID_HEADER is a header field used to correlate OCM events
	OPERATION_ID_HEADER = "X-Operation-Id"
	// CLUSTERS_V1_PATH is a path to the OCM clusters service
	CLUSTERS_V1_PATH = "/api/clusters_mgmt/v1/clusters"
	// UPGRADEPOLICIES_V1_PATH is a sub-path to the OCM upgrade policies service
	UPGRADEPOLICIES_V1_PATH = "upgrade_policies"
	// STATE_V1_PATH sub-path to the policy state service
	STATE_V1_PATH = "state"
)

var (
	// ErrClusterIdNotFound is an error describing the cluster ID can not be found
	ErrClusterIdNotFound = fmt.Errorf("cluster ID can't be found")
)

// OcmClient enables an implementation of an ocm client
//go:generate mockgen -destination=mocks/client.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/ocm OcmClient
type OcmClient interface {
	GetCluster() (*ClusterInfo, error)
	GetClusterUpgradePolicies(clusterId string) (*UpgradePolicyList, error)
	GetClusterUpgradePolicyState(policyId string, clusterId string) (*UpgradePolicyState, error)
	SetState(value string, description string, policyId string, clusterId string) error
}

type ocmClient struct {
	// Cluster k8s client
	client client.Client
	// Base OCM API Url
	ocmBaseUrl *url.URL
	// HTTP client used for API queries (TODO: remove in favour of OCM SDK)
	httpClient *resty.Client
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

// Read cluster info from OCM
func (s *ocmClient) GetCluster() (*ClusterInfo, error) {

	// fetch the clusterversion, which contains the internal ID
	cv := &configv1.ClusterVersion{}
	err := s.client.Get(context.TODO(), types.NamespacedName{Name: "version"}, cv)
	if err != nil {
		return nil, fmt.Errorf("can't get clusterversion: %v", err)
	}
	externalID := cv.Spec.ClusterID

	csUrl, err := url.Parse(s.ocmBaseUrl.String())
	if err != nil {
		return nil, fmt.Errorf("can't parse OCM API url: %v", err)
	}
	csUrl.Path = path.Join(csUrl.Path, CLUSTERS_V1_PATH)

	response, err := s.httpClient.R().
		SetQueryParams(map[string]string{
			"page":   "1",
			"size":   "1",
			"search": fmt.Sprintf("external_id = '%s'", externalID),
		}).
		SetResult(&ClusterList{}).
		ExpectContentType("application/json").
		Get(csUrl.String())

	if err != nil {
		return nil, fmt.Errorf("can't query OCM cluster service: %v", err)
	}

	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return nil, fmt.Errorf("received error code %v, operation id '%v'", response.StatusCode(), operationId)
	}

	listResponse := response.Result().(*ClusterList)
	if listResponse.Size != 1 || len(listResponse.Items) != 1 {
		return nil, ErrClusterIdNotFound
	}

	return &listResponse.Items[0], nil
}

// Queries and returns the Upgrade Policy from Cluster Services
func (s *ocmClient) GetClusterUpgradePolicies(clusterId string) (*UpgradePolicyList, error) {

	upUrl, err := url.Parse(s.ocmBaseUrl.String())
	if err != nil {
		return nil, fmt.Errorf("can't read OCM API url: %v", err)
	}
	upUrl.Path = path.Join(upUrl.Path, CLUSTERS_V1_PATH, clusterId, UPGRADEPOLICIES_V1_PATH)

	response, err := s.httpClient.R().
		SetResult(&UpgradePolicyList{}).
		ExpectContentType("application/json").
		Get(upUrl.String())

	if err != nil {
		return nil, fmt.Errorf("can't send notification: %v", err)
	}
	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return nil, fmt.Errorf("received error code '%v' from OCM upgrade policy service, operation id '%v'", response.StatusCode(), operationId)
	}

	upgradeResponse := response.Result().(*UpgradePolicyList)
	return upgradeResponse, nil
}

// Send a notification of state
func (s *ocmClient) SetState(value string, description string, policyId string, clusterId string) error {

	policyState := UpgradePolicyStateRequest{
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

// Queries and returns the Upgrade Policy state from Cluster Services
func (s *ocmClient) GetClusterUpgradePolicyState(policyId string, clusterId string) (*UpgradePolicyState, error) {

	upUrl, err := url.Parse(s.ocmBaseUrl.String())
	if err != nil {
		return nil, fmt.Errorf("can't read OCM API url: %v", err)
	}
	upUrl.Path = path.Join(upUrl.Path, CLUSTERS_V1_PATH, clusterId, UPGRADEPOLICIES_V1_PATH, policyId, STATE_V1_PATH)

	response, err := s.httpClient.R().
		SetResult(&UpgradePolicyState{}).
		ExpectContentType("application/json").
		Get(upUrl.String())

	if err != nil {
		return nil, fmt.Errorf("can't send notification: %v", err)
	}
	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return nil, fmt.Errorf("received error code '%v' from OCM upgrade policy service, operation id '%v'", response.StatusCode(), operationId)
	}

	stateResponse := response.Result().(*UpgradePolicyState)
	return stateResponse, nil
}

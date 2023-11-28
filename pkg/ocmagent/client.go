package ocmagent

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/openshift/managed-upgrade-operator/pkg/ocm"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	OCM_AGENT_SERVICE_URL = "ocm-agent.openshift-ocm-agent-operator.svc.cluster.local"
	OCM_AGENT_SERVICE_PORT = 8081
	// OPERATION_ID_HEADER is a header field used to correlate OCM events
	OPERATION_ID_HEADER = "X-Operation-Id"
	// UPGRADEPOLICIES_V1_PATH is a sub-path to the OCM upgrade policies service
	UPGRADEPOLICIES_V1_PATH = "upgrade_policies"
	// STATE_V1_PATH sub-path to the policy state service
	STATE_V1_PATH = "state"
)

var log = logf.Log.WithName("ocm-client")

var (
	// ErrClusterIdNotFound is an error describing the cluster ID can not be found
	ErrClusterIdNotFound = fmt.Errorf("OCM did not return a valid cluster ID: pull-secret may be invalid OR cluster's owner is disabled/banned in OCM")
)

type ocmClient struct {
	// Cluster k8s client
	client client.Client
	// Base OCM API Url
	ocmBaseUrl *url.URL
	// HTTP client used for API queries (TODO: remove in favour of OCM SDK)
	httpClient *resty.Client
}

type ocmRoundTripper struct {}

func (ort *ocmRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	transport := http.Transport{
		TLSHandshakeTimeout: time.Second * 5,
	}
	return transport.RoundTrip(req)
}

// Read cluster info from OCM
func (s *ocmClient) GetCluster() (*ocm.ClusterInfo, error) {
	csUrl, err := url.Parse(s.ocmBaseUrl.String())
	if err != nil {
		return nil, fmt.Errorf("can't parse OCM API url: %v", err)
	}

	// GET ocm-agent.svc.local/
	response, err := s.httpClient.R().
		SetResult(&ocm.ClusterInfo{}).
		ExpectContentType("application/json").
		Get(csUrl.String())

	if err != nil {
		return nil, fmt.Errorf("can't query OCM cluster service: request to '%v' returned error '%v'", csUrl.String(), err)
	}

	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return nil, fmt.Errorf("request to '%v' received error code %v, operation id '%v'", csUrl.String(), response.StatusCode(), operationId)
	}

	log.Info(fmt.Sprintf("request to '%v' received response code %v, operation id: '%v'", csUrl.String(), response.StatusCode(), operationId))

	clusterInfo := response.Result().(*ocm.ClusterInfo)
	return clusterInfo, nil
}

// Queries and returns the Upgrade Policy from Cluster Services
func (s *ocmClient) GetClusterUpgradePolicies(clusterId string) (*ocm.UpgradePolicyList, error) {
	upUrl, err := url.Parse(s.ocmBaseUrl.String())
	if err != nil {
		return nil, fmt.Errorf("can't read OCM API url: %v", err)
	}
	upUrl.Path = path.Join(UPGRADEPOLICIES_V1_PATH)

	response, err := s.httpClient.R().
		SetResult(&[]ocm.UpgradePolicy{}).
		ExpectContentType("application/json").
		Get(upUrl.String())

	if err != nil {
		return nil, fmt.Errorf("can't pull upgrade policies: request to '%v' returned error '%v'", upUrl.String(), err)
	}
	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return nil, fmt.Errorf("request to '%v' received error code '%v' from OCM upgrade policy service, operation id '%v'", upUrl.String(), response.StatusCode(), operationId)
	}

	log.Info(fmt.Sprintf("request to '%v' received response code '%v' from OCM upgrade policy service, operation id: '%v'", upUrl.String(), response.StatusCode(), operationId))

	upgradeResponse := response.Result().(*[]ocm.UpgradePolicy)

	return &ocm.UpgradePolicyList{
		Kind: "UpgradePolicyList",
		Page: 1,
		Size: int64(len(*upgradeResponse)),
		Total: int64(len(*upgradeResponse)),
		Items: *upgradeResponse,
	}, nil
}

// Send a notification of state
func (s *ocmClient) SetState(value string, description string, policyId string, clusterId string) error {

	policyState := ocm.UpgradePolicyStateRequest{
		Value:       string(value),
		Description: description,
	}

	// Create the URL path to send to
	reqUrl, err := url.Parse(s.ocmBaseUrl.String())
	if err != nil {
		return fmt.Errorf("can't read OCM API url: %v", err)
	}
	reqUrl.Path = path.Join(UPGRADEPOLICIES_V1_PATH, policyId, STATE_V1_PATH)

	response, err := s.httpClient.R().
		SetHeader("Content-Type", "application/json").
		SetBody(policyState).
		ExpectContentType("application/json").
		Patch(reqUrl.String())

	if err != nil {
		return fmt.Errorf("can't set upgrade policy state: request to '%v' returned error '%v'", reqUrl.String(), err)
	}
	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return fmt.Errorf("request to '%v' received error code %v, operation id '%v'", reqUrl.String(), response.StatusCode(), operationId)
	}

	return nil
}

// Queries and returns the Upgrade Policy state from Cluster Services
func (s *ocmClient) GetClusterUpgradePolicyState(policyId string, clusterId string) (*ocm.UpgradePolicyState, error) {

	upUrl, err := url.Parse(s.ocmBaseUrl.String())
	if err != nil {
		return nil, fmt.Errorf("can't read OCM API url: %v", err)
	}
	upUrl.Path = path.Join(UPGRADEPOLICIES_V1_PATH, policyId, STATE_V1_PATH)

	response, err := s.httpClient.R().
		SetResult(&ocm.UpgradePolicyState{}).
		ExpectContentType("application/json").
		Get(upUrl.String())

	if err != nil {
		return nil, fmt.Errorf("can't pull upgrade policy state: request to '%v' returned error '%v'", upUrl.String(), err)
	}
	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return nil, fmt.Errorf("received error code '%v' from OCM upgrade policy service, operation id '%v'", response.StatusCode(), operationId)
	}

	stateResponse := response.Result().(*ocm.UpgradePolicyState)
	return stateResponse, nil
}

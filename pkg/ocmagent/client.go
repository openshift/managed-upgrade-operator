package ocmagent

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/go-resty/resty/v2"
	servicelogsv1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	"github.com/openshift/managed-upgrade-operator/pkg/ocm"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	OCM_AGENT_SERVICE_URL  = "ocm-agent.openshift-ocm-agent-operator.svc.cluster.local"
	OCM_AGENT_SERVICE_PORT = 8081
	// OPERATION_ID_HEADER is a header field used to correlate OCM events
	OPERATION_ID_HEADER = "X-Operation-Id"
	// UPGRADEPOLICIES_PATH is a sub-path to the OCM upgrade policies service
	UPGRADEPOLICIES_PATH = "upgrade_policies"
	// STATE_V1_PATH sub-path to the policy state service
	STATE_V1_PATH = "state"

	// SERVICELOG_LOG_TYPE is the log type sent from MUO
	SERVICELOG_LOG_TYPE = "Cluster Updates"
	// SERVICELOG_SERVICE_NAME is the name of the service reporting the log
	SERVICELOG_SERVICE_NAME = "RedHat Managed Upgrade Notifications"
	// SERVICELOG_INTERNAL_ONLY defines if the log is internal or not
	SERVICELOG_INTERNAL_ONLY = false
)

var log = logf.Log.WithName("ocm-client")

type ocmClient struct {
	// Cluster k8s client
	client client.Client
	// Base OCM API Url
	ocmBaseUrl *url.URL
	// HTTP client used for API queries (TODO: remove in favour of OCM SDK)
	httpClient *resty.Client
	// OCM SDK Client
	sdkClient *SdkClient
}

// // ServiceLog is the internal representation of a service log
// type ServiceLog struct {
// 	Severity     string
// 	ServiceName  string
// 	Summary      string
// 	Description  string
// 	InternalOnly bool
// }

type ocmRoundTripper struct{}

func (ort *ocmRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	transport := http.Transport{
		TLSHandshakeTimeout: time.Second * 5,
	}
	return transport.RoundTrip(req)
}

// Read cluster info from OCM
func (s *ocmClient) GetCluster() (*ocm.ClusterInfo, error) {
	apiUrl, err := parseOcmBaseUrl(s.ocmBaseUrl)
	if err != nil {
		return nil, fmt.Errorf("can't parse OCM API url: %v", err)
	}

	// GET ocm-agent.svc.local/
	response, err := s.httpClient.R().
		SetResult(&ocm.ClusterInfo{}).
		ExpectContentType("application/json").
		Get(apiUrl.String())

	if err != nil {
		return nil, fmt.Errorf("can't query OCM cluster service: request to '%v' returned error '%v'", apiUrl.String(), err)
	}

	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return nil, fmt.Errorf("request to '%v' received error code %v, operation id '%v'", apiUrl.String(), response.StatusCode(), operationId)
	}

	log.Info(fmt.Sprintf("request to '%v' received response code %v, operation id: '%v'", apiUrl.String(), response.StatusCode(), operationId))

	clusterInfo := response.Result().(*ocm.ClusterInfo)
	return clusterInfo, nil
}

// Queries and returns the Upgrade Policy from Cluster Services
func (s *ocmClient) GetClusterUpgradePolicies(clusterId string) (*ocm.UpgradePolicyList, error) {
	apiUrl, err := parseOcmBaseUrl(s.ocmBaseUrl)
	if err != nil {
		return nil, fmt.Errorf("can't read OCM API url: %v", err)
	}
	apiUrl.Path = path.Join(UPGRADEPOLICIES_PATH)

	response, err := s.httpClient.R().
		SetResult(&[]ocm.UpgradePolicy{}).
		ExpectContentType("application/json").
		Get(apiUrl.String())

	if err != nil {
		return nil, fmt.Errorf("can't pull upgrade policies: request to '%v' returned error '%v'", apiUrl.String(), err)
	}
	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return nil, fmt.Errorf("request to '%v' received error code '%v' from OCM upgrade policy service, operation id '%v'", apiUrl.String(), response.StatusCode(), operationId)
	}

	log.Info(fmt.Sprintf("request to '%v' received response code '%v' from OCM upgrade policy service, operation id: '%v'", apiUrl.String(), response.StatusCode(), operationId))

	upgradeResponse := response.Result().(*[]ocm.UpgradePolicy)

	return &ocm.UpgradePolicyList{
		Kind:  "UpgradePolicyList",
		Page:  1,
		Size:  int64(len(*upgradeResponse)),
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
	apiUrl, err := parseOcmBaseUrl(s.ocmBaseUrl)
	if err != nil {
		return fmt.Errorf("can't read OCM API url: %v", err)
	}
	apiUrl.Path = path.Join(UPGRADEPOLICIES_PATH, policyId, STATE_V1_PATH)

	response, err := s.httpClient.R().
		SetHeader("Content-Type", "application/json").
		SetBody(policyState).
		ExpectContentType("application/json").
		Patch(apiUrl.String())

	if err != nil {
		return fmt.Errorf("can't set upgrade policy state: request to '%v' returned error '%v'", apiUrl.String(), err)
	}
	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return fmt.Errorf("request to '%v' received error code %v, operation id '%v'", apiUrl.String(), response.StatusCode(), operationId)
	}

	return nil
}

// Queries and returns the Upgrade Policy state from Cluster Services
func (s *ocmClient) GetClusterUpgradePolicyState(policyId string, clusterId string) (*ocm.UpgradePolicyState, error) {

	apiUrl, err := parseOcmBaseUrl(s.ocmBaseUrl)
	if err != nil {
		return nil, fmt.Errorf("can't read OCM API url: %v", err)
	}
	apiUrl.Path = path.Join(UPGRADEPOLICIES_PATH, policyId, STATE_V1_PATH)

	response, err := s.httpClient.R().
		SetResult(&ocm.UpgradePolicyState{}).
		ExpectContentType("application/json").
		Get(apiUrl.String())

	if err != nil {
		return nil, fmt.Errorf("can't pull upgrade policy state: request to '%v' returned error '%v'", apiUrl.String(), err)
	}
	operationId := response.Header().Get(OPERATION_ID_HEADER)
	if response.IsError() {
		return nil, fmt.Errorf("received error code '%v' from OCM upgrade policy service, operation id '%v'", response.StatusCode(), operationId)
	}

	stateResponse := response.Result().(*ocm.UpgradePolicyState)
	return stateResponse, nil
}

// PostServiceLog allows to send a generic servicelog to a cluster.
func (s *ocmClient) PostServiceLog(sl *ocm.ServiceLog, description string) error {
	cluster, err := s.GetCluster()
	if err != nil {
		return fmt.Errorf("failed to retrieve internal cluster ID from OCM: %v", err)
	}
	builder := &servicelogsv1.LogEntryBuilder{}

	// We set standard fields here which are common across different ServiceLogs sent
	builder.Severity(servicelogsv1.Severity(servicelogsv1.SeverityInfo))
	builder.InternalOnly(SERVICELOG_INTERNAL_ONLY)
	builder.ServiceName(SERVICELOG_SERVICE_NAME)
	builder.LogType(SERVICELOG_LOG_TYPE)
	builder.Description(description)
	builder.ClusterID(cluster.Id)

	// Else refer to the values in 'sl'
	builder.Summary(sl.Summary)

	le, err := builder.Build()
	if err != nil {
		return fmt.Errorf("could not create post request (SL): %w", err)
	}

	request := s.sdkClient.conn.ServiceLogs().V1().ClusterLogs().Add()
	request = request.Body(le)

	response, err := request.Send()
	if err != nil || response.Error() != nil {
		return fmt.Errorf("could not post service log %s: %v", sl.Summary, err)
	}

	fmt.Printf("Successfully sent servicelog: %s", sl.Summary)

	return nil
}

func parseOcmBaseUrl(ocmBaseUrl *url.URL) (*url.URL, error) {
	url, err := url.Parse(ocmBaseUrl.String())
	return url, err
}

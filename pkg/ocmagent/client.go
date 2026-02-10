package ocmagent

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"

	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
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
	// OCM SDK connection for all HTTP operations
	conn *sdk.Connection
}

// clusterInfoJSON is a temporary type for unmarshaling ocm-agent JSON responses
type clusterInfoJSON struct {
	Id      string `json:"id"`
	Version struct {
		Id           string `json:"id"`
		ChannelGroup string `json:"channel_group"`
	} `json:"version"`
	NodeDrainGracePeriod struct {
		Value int64  `json:"value"`
		Unit  string `json:"unit"`
	} `json:"node_drain_grace_period"`
}

// Read cluster info from OCM via ocm-agent and return SDK type
func (s *ocmClient) GetCluster() (*cmv1.Cluster, error) {
	apiUrl, err := parseOcmBaseUrl(s.ocmBaseUrl)
	if err != nil {
		return nil, fmt.Errorf("can't parse OCM API url: %v", err)
	}

	// GET ocm-agent.svc.local/ (root path)
	response, err := s.conn.Get().
		Path("/").
		Send()

	if err != nil {
		return nil, fmt.Errorf("can't query OCM cluster service: request to '%v' returned error '%v'", apiUrl.String(), err)
	}

	operationId := response.Header(OPERATION_ID_HEADER)
	statusCode := response.Status()

	if statusCode >= 400 {
		return nil, fmt.Errorf("request to '%v' received error code %v, operation id '%v'", apiUrl.String(), statusCode, operationId)
	}

	log.Info(fmt.Sprintf("request to '%v' received response code %v, operation id: '%v'", apiUrl.String(), statusCode, operationId))

	// Unmarshal simplified JSON from ocm-agent
	var clusterInfo clusterInfoJSON
	if err := json.Unmarshal(response.Bytes(), &clusterInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster info response: %v", err)
	}

	// Build SDK cluster type from JSON
	cluster, err := cmv1.NewCluster().
		ID(clusterInfo.Id).
		Version(cmv1.NewVersion().
			ID(clusterInfo.Version.Id).
			ChannelGroup(clusterInfo.Version.ChannelGroup)).
		NodeDrainGracePeriod(cmv1.NewValue().
			Value(float64(clusterInfo.NodeDrainGracePeriod.Value)).
			Unit(clusterInfo.NodeDrainGracePeriod.Unit)).
		Build()

	if err != nil {
		return nil, fmt.Errorf("failed to build cluster SDK type: %v", err)
	}

	return cluster, nil
}

// upgradePolicyJSON is a temporary type for unmarshaling ocm-agent JSON responses
type upgradePolicyJSON struct {
	Id                  string `json:"id"`
	Kind                string `json:"kind"`
	Href                string `json:"href"`
	Schedule            string `json:"schedule"`
	ScheduleType        string `json:"schedule_type"`
	UpgradeType         string `json:"upgrade_type"`
	Version             string `json:"version"`
	NextRun             string `json:"next_run"`
	PrevRun             string `json:"prev_run"`
	ClusterId           string `json:"cluster_id"`
	CapacityReservation *bool  `json:"capacity_reservation"`
}

// Queries and returns the Upgrade Policy from Cluster Services via ocm-agent using SDK typed API
func (s *ocmClient) GetClusterUpgradePolicies(clusterId string) (*cmv1.UpgradePoliciesListResponse, error) {
	// Try using SDK typed API - the connection is already pointing at ocm-agent
	// ocm-agent should proxy to OCM and return compatible responses
	response, err := s.conn.ClustersMgmt().V1().
		Clusters().
		Cluster(clusterId).
		UpgradePolicies().
		List().
		Page(1).
		Size(1).
		Send()

	if err != nil {
		return nil, fmt.Errorf("can't pull upgrade policies for cluster %s: %w", clusterId, err)
	}

	operationId := response.Header().Get(OPERATION_ID_HEADER)
	statusCode := response.Status()

	// Construct full URL for logging
	apiUrl, urlErr := parseOcmBaseUrl(s.ocmBaseUrl)
	if urlErr == nil {
		apiUrl.Path = path.Join(UPGRADEPOLICIES_PATH)
	}

	if statusCode >= 400 {
		logUrl := "ocm-agent"
		if urlErr == nil {
			logUrl = apiUrl.String()
		}
		return nil, fmt.Errorf("request to '%v' received error code %v from OCM upgrade policy service, operation id '%v'", logUrl, statusCode, operationId)
	}

	if urlErr == nil {
		log.Info(fmt.Sprintf("request to '%v' received response code %v from OCM upgrade policy service, operation id: '%v'", apiUrl.String(), statusCode, operationId))
	}

	return response, nil
}

// Send a notification of state
func (s *ocmClient) SetState(value string, description string, policyId string, clusterId string) error {
	// Build the state update using SDK builder
	stateUpdate, err := cmv1.NewUpgradePolicyState().
		Value(cmv1.UpgradePolicyStateValue(value)).
		Description(description).
		Build()

	if err != nil {
		return fmt.Errorf("failed to build policy state: %v", err)
	}

	// Marshal request body
	bodyBytes, err := json.Marshal(stateUpdate)
	if err != nil {
		return fmt.Errorf("failed to marshal policy state: %v", err)
	}

	// Build API path
	apiPath := path.Join("/", UPGRADEPOLICIES_PATH, policyId, STATE_V1_PATH)

	// Construct full URL for logging
	apiUrl, err := parseOcmBaseUrl(s.ocmBaseUrl)
	if err != nil {
		return fmt.Errorf("can't read OCM API url: %v", err)
	}
	apiUrl.Path = path.Join(UPGRADEPOLICIES_PATH, policyId, STATE_V1_PATH)

	// Make PATCH request using SDK
	response, err := s.conn.Patch().
		Path(apiPath).
		Bytes(bodyBytes).
		Send()

	if err != nil {
		return fmt.Errorf("can't set upgrade policy state: request to '%v' returned error '%v'", apiUrl.String(), err)
	}

	operationId := response.Header(OPERATION_ID_HEADER)
	statusCode := response.Status()

	if statusCode >= 400 {
		return fmt.Errorf("request to '%v' received error code %v, operation id '%v'", apiUrl.String(), statusCode, operationId)
	}

	return nil
}

// upgradePolicyStateJSON is a temporary type for unmarshaling ocm-agent JSON responses
type upgradePolicyStateJSON struct {
	Kind        string `json:"kind"`
	Href        string `json:"href"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

// Queries and returns the Upgrade Policy state from Cluster Services via ocm-agent and returns SDK type
func (s *ocmClient) GetClusterUpgradePolicyState(policyId string, clusterId string) (*cmv1.UpgradePolicyState, error) {

	// Build API path
	apiPath := path.Join("/", UPGRADEPOLICIES_PATH, policyId, STATE_V1_PATH)

	// Construct full URL for logging
	apiUrl, err := parseOcmBaseUrl(s.ocmBaseUrl)
	if err != nil {
		return nil, fmt.Errorf("can't read OCM API url: %v", err)
	}
	apiUrl.Path = path.Join(UPGRADEPOLICIES_PATH, policyId, STATE_V1_PATH)

	// Make GET request using SDK
	response, err := s.conn.Get().
		Path(apiPath).
		Send()

	if err != nil {
		return nil, fmt.Errorf("can't pull upgrade policy state: request to '%v' returned error '%v'", apiUrl.String(), err)
	}

	operationId := response.Header(OPERATION_ID_HEADER)
	statusCode := response.Status()

	if statusCode >= 400 {
		return nil, fmt.Errorf("received error code %v from OCM upgrade policy service, operation id '%v'", statusCode, operationId)
	}

	// Unmarshal simplified JSON from ocm-agent
	var stateResponse upgradePolicyStateJSON
	if err := json.Unmarshal(response.Bytes(), &stateResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal upgrade policy state response: %v", err)
	}

	// Build SDK upgrade policy state from JSON
	state, err := cmv1.NewUpgradePolicyState().
		Value(cmv1.UpgradePolicyStateValue(stateResponse.Value)).
		Description(stateResponse.Description).
		Build()

	if err != nil {
		return nil, fmt.Errorf("failed to build upgrade policy state SDK type: %v", err)
	}

	return state, nil
}

// PostServiceLog allows to send a generic servicelog to a cluster.
func (s *ocmClient) PostServiceLog(sl *ocm.ServiceLog, description string) error {
	cluster, err := s.GetCluster()
	if err != nil {
		return fmt.Errorf("failed to retrieve internal cluster ID from OCM: %v", err)
	}
	builder := &servicelogsv1.LogEntryBuilder{}

	// Set the severity
	if sl.Severity != "" {
		builder.Severity(sl.Severity)
	} else {
		builder.Severity(servicelogsv1.SeverityInfo)
	}
	// We set standard fields here which are common across different ServiceLogs sent
	builder.InternalOnly(SERVICELOG_INTERNAL_ONLY)
	builder.ServiceName(SERVICELOG_SERVICE_NAME)
	builder.LogType(SERVICELOG_LOG_TYPE)
	builder.Description(description)
	builder.ClusterID(cluster.ID())

	// Else refer to the values in 'sl'
	builder.Summary(sl.Summary)

	le, err := builder.Build()
	if err != nil {
		return fmt.Errorf("could not create post request (SL): %w", err)
	}

	request := s.conn.ServiceLogs().V1().ClusterLogs().Add()
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

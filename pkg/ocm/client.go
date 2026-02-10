package ocm

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	servicelogsv1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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

	// SERVICELOG_LOG_TYPE is the log type sent from MUO
	SERVICELOG_LOG_TYPE = "Cluster Updates"
	// SERVICELOG_SERVICE_NAME is the name of the service reporting the log
	SERVICELOG_SERVICE_NAME = "RedHat Managed Upgrade Notifications"
	// SERVICELOG_INTERNAL_ONLY defines if the log is internal or not
	SERVICELOG_INTERNAL_ONLY = false

	// TLS_HANDSHAKE_TIMEOUT is the timeout for TLS handshake
	// Increased from default 10s to 30s to handle high-latency networks and proxy environments
	TLS_HANDSHAKE_TIMEOUT = 30 * time.Second
)

var log = logf.Log.WithName("ocm-client")

var (
	// ErrClusterIdNotFound is an error describing the cluster ID can not be found
	ErrClusterIdNotFound = fmt.Errorf("OCM did not return a valid cluster ID: pull-secret may be invalid OR cluster's owner is disabled/banned in OCM")
)

// OcmClient enables an implementation of an ocm client
//
//go:generate mockgen -destination=mocks/client.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/ocm OcmClient
type OcmClient interface {
	GetCluster() (*cmv1.Cluster, error)
	GetClusterUpgradePolicies(clusterId string) (*cmv1.UpgradePoliciesListResponse, error)
	GetClusterUpgradePolicyState(policyId string, clusterId string) (*cmv1.UpgradePolicyState, error)
	PostServiceLog(sl *ServiceLog, description string) error
	SetState(value string, description string, policyId string, clusterId string) error
}

type ocmClient struct {
	// Cluster k8s client
	client client.Client
	// Base OCM API Url
	ocmBaseUrl *url.URL
	// OCM SDK connection for all HTTP operations
	conn *sdk.Connection
}

// ServiceLog is the internal representation of a service log
type ServiceLog struct {
	Severity      servicelogsv1.Severity
	ServiceName   string
	Summary       string
	Description   string
	InternalOnly  bool
	DocReferences string
}

// Read cluster info from OCM using SDK typed API
func (s *ocmClient) GetCluster() (*cmv1.Cluster, error) {

	// fetch the clusterversion, which contains the external ID
	cv := &configv1.ClusterVersion{}
	err := s.client.Get(context.TODO(), types.NamespacedName{Name: "version"}, cv)
	if err != nil {
		return nil, fmt.Errorf("can't get clusterversion: %v", err)
	}
	externalID := cv.Spec.ClusterID

	// Use SDK typed API to search for cluster by external_id
	clustersSearch := fmt.Sprintf("external_id = '%s'", externalID)
	clustersListResponse, err := s.conn.ClustersMgmt().V1().Clusters().
		List().
		Search(clustersSearch).
		Size(1).
		Send()

	if err != nil {
		return nil, fmt.Errorf("can't query OCM cluster service for external_id '%s': %w", externalID, err)
	}

	operationId := clustersListResponse.Header().Get(OPERATION_ID_HEADER)
	statusCode := clustersListResponse.Status()

	// Construct full URL for logging
	csUrl, err := url.Parse(s.ocmBaseUrl.String())
	if err != nil {
		return nil, fmt.Errorf("can't parse OCM API url: %v", err)
	}
	csUrl.Path = path.Join(csUrl.Path, CLUSTERS_V1_PATH)
	csUrl.RawQuery = fmt.Sprintf("search=%s&size=1", url.QueryEscape(clustersSearch))

	if statusCode >= 400 {
		return nil, fmt.Errorf("request to '%v' received error code %v, operation id '%v'", csUrl.String(), statusCode, operationId)
	}

	log.Info(fmt.Sprintf("request to '%v' received response code %v, operation id: '%v'", csUrl.String(), statusCode, operationId))

	// Check if exactly one cluster was found
	clustersTotal := clustersListResponse.Total()
	if clustersTotal != 1 {
		return nil, ErrClusterIdNotFound
	}

	// Return the SDK cluster object
	cluster := clustersListResponse.Items().Get(0)
	return cluster, nil
}

// Queries and returns the Upgrade Policy from Cluster Services using SDK typed API
func (s *ocmClient) GetClusterUpgradePolicies(clusterId string) (*cmv1.UpgradePoliciesListResponse, error) {

	// Use SDK typed API to get upgrade policies
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
	upUrl, err := url.Parse(s.ocmBaseUrl.String())
	if err != nil {
		return nil, fmt.Errorf("can't read OCM API url: %v", err)
	}
	upUrl.Path = path.Join(upUrl.Path, CLUSTERS_V1_PATH, clusterId, UPGRADEPOLICIES_V1_PATH)
	upUrl.RawQuery = "page=1&size=1"

	if statusCode >= 400 {
		return nil, fmt.Errorf("request to '%v' received error code %v from OCM upgrade policy service, operation id '%v'", upUrl.String(), statusCode, operationId)
	}

	log.Info(fmt.Sprintf("request to '%v' received response code %v from OCM upgrade policy service, operation id: '%v'", upUrl.String(), statusCode, operationId))

	return response, nil
}

// Send a notification of state using SDK typed API with builder pattern
func (s *ocmClient) SetState(value string, description string, policyId string, clusterId string) error {

	// Build the state update using SDK builder
	stateUpdate, err := cmv1.NewUpgradePolicyState().
		Value(cmv1.UpgradePolicyStateValue(value)).
		Description(description).
		Build()

	if err != nil {
		return fmt.Errorf("failed to build policy state: %v", err)
	}

	// Construct full URL for logging
	reqUrl, err := url.Parse(s.ocmBaseUrl.String())
	if err != nil {
		return fmt.Errorf("can't read OCM API url: %v", err)
	}
	reqUrl.Path = path.Join(reqUrl.Path, CLUSTERS_V1_PATH, clusterId, UPGRADEPOLICIES_V1_PATH, policyId, STATE_V1_PATH)

	// Use SDK typed API to update state
	response, err := s.conn.ClustersMgmt().V1().
		Clusters().
		Cluster(clusterId).
		UpgradePolicies().
		UpgradePolicy(policyId).
		State().
		Update().
		Body(stateUpdate).
		Send()

	if err != nil {
		return fmt.Errorf("can't set upgrade policy state: request to '%v' returned error '%v'", reqUrl.String(), err)
	}

	operationId := response.Header().Get(OPERATION_ID_HEADER)
	statusCode := response.Status()

	if statusCode >= 400 {
		return fmt.Errorf("request to '%v' received error code %v, operation id '%v'", reqUrl.String(), statusCode, operationId)
	}

	return nil
}

// Queries and returns the Upgrade Policy state from Cluster Services using SDK typed API
func (s *ocmClient) GetClusterUpgradePolicyState(policyId string, clusterId string) (*cmv1.UpgradePolicyState, error) {

	// Use SDK typed API to get policy state
	response, err := s.conn.ClustersMgmt().V1().
		Clusters().
		Cluster(clusterId).
		UpgradePolicies().
		UpgradePolicy(policyId).
		State().
		Get().
		Send()

	if err != nil {
		return nil, fmt.Errorf("can't pull upgrade policy state: %w", err)
	}

	operationId := response.Header().Get(OPERATION_ID_HEADER)
	statusCode := response.Status()

	if statusCode >= 400 {
		return nil, fmt.Errorf("received error code %v from OCM upgrade policy service, operation id '%v'", statusCode, operationId)
	}

	return response.Body(), nil
}

// PostServiceLog allows to send a generic servicelog to a cluster.
func (s *ocmClient) PostServiceLog(sl *ServiceLog, description string) error {
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
	if len(sl.DocReferences) > 0 {
		builder.DocReferences(sl.DocReferences)
	}

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

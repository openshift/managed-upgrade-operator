package upgrade_config_manager

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/blang/semver"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/util"
)

const (
	OCM_API_URL                = "https://api.stage.openshift.com/"
	CLUSTERS_V1_PATH           = "/api/clusters_mgmt/v1/clusters"
	UPGRADEPOLICIES_V1_PATH    = "upgrade_policies"
	UPGRADECONFIG_CR_NAME      = "osd-upgrade-config"
	UPGRADECONFIG_CR_NAMESPACE = "openshift-managed-upgrade-operator"
)

var log = logf.Log.WithName("managed-upgrade-operator")

// TotalAccountWatcher global var for TotalAccountWatcher
var UpgradeConfigManager *upgradeConfigManager

type upgradeConfigManager struct {
	watchInterval time.Duration
	client        client.Client
	clusterID     string
	ocm           *sdk.Connection
	httpClient    *http.Client
}

type clusterUpgradeList struct {
	Kind  string           `json:"kind"`
	Page  int64            `json:"page"`
	Size  int64            `json:"size"`
	Total int64            `json:"total"`
	Items []clusterUpgrade `json:"items"`
}

type clusterUpgrade struct {
	Id                   int64                `json:"id"`
	Kind                 string               `json:"kind"`
	Href                 string               `json:"href"`
	Schedule             string               `json:"schedule"`
	ScheduleType         string               `json:"schedule_type"`
	UpgradeType          string               `json:"upgrade_type"`
	Version              upgradeVersion       `json:"version"`
	NextRun              string               `json:"next_run"`
	PrevRun              string               `json:"prev_run"`
	NodeDrainGracePeriod nodeDrainGracePeriod `json:"node_drain_grace_period"`
}

type clusterList struct {
	Kind  string           `json:"kind"`
	Page  int64            `json:"page"`
	Size  int64            `json:"size"`
	Total int64            `json:"total"`
	Items []cluster `json:"items"`
}

type cluster struct {
	Id   string `json:"id"`
}

type upgradeVersion struct {
	Id           string `json:"id"`
	ChannelGroup string `json:"channel_group"`
}

type nodeDrainGracePeriod struct {
	Value int64  `json:"value"`
	Units string `json:"units"`
}

type ocmRoundTripper struct {
	authorization util.AccessToken
}

// Initialize creates a global instance of the UpgradeConfigManager
func Initialize(client client.Client, watchInterval time.Duration) {
	log.Info("Initializing the upgradeConfigManager")

	var conn *sdk.Connection = nil
	// TODO: Commenting below out until we can use OCM for AccessToken auth
	////Set up the OCM client here
	//conn, err := setupOCMConnection(client)
	//if err != nil {
	//	log.Error(err, "failed to set up OCM connection")
	//	return
	//}

	// Fetch the cluster AccessToken
	accessToken, err := util.GetAccessToken(client)
	if err != nil {
		log.Error(err, "failed to retrieve cluster access token")
		return
	}
	// Set up the HTTP client using the token
	httpClient := &http.Client{
		Transport: &ocmRoundTripper{
			authorization: *accessToken,
		},
	}

	UpgradeConfigManager = NewUpgradeConfigManager(client, watchInterval, conn, httpClient)
	err = UpgradeConfigManager.RefreshUpgradeConfig(log)
	if err != nil {
		log.Error(err, "failed to refresh upgrade config")
		return
	}
}

func (ort *ocmRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	authVal := fmt.Sprintf("AccessToken %s:%s", ort.authorization.ClusterId, ort.authorization.PullSecret)
	req.Header.Add("Authorization", authVal)
	transport := http.Transport{
		TLSHandshakeTimeout: time.Second * 5,
	}
	return transport.RoundTrip(req)
}

// NewTotalAccountWatcher returns a new instance of the TotalAccountWatcher interface
func NewUpgradeConfigManager(
	client client.Client,
	watchInterval time.Duration,
	ocmConnection *sdk.Connection,
	httpClient *http.Client,
) *upgradeConfigManager {
	return &upgradeConfigManager{
		watchInterval: watchInterval,
		client:        client,
		ocm:           ocmConnection,
		httpClient:    httpClient,
	}
}

// UpgradeConfigManager will trigger X every `scanInternal` and only stop if the operator is killed or a
// message is sent on the stopCh
func (s *upgradeConfigManager) Start(log logr.Logger, stopCh <-chan struct{}) {
	log.Info("Starting the upgradeConfigManager")
	for {
		select {
		case <-time.After(s.watchInterval):
			err := s.RefreshUpgradeConfig(log)
			if err != nil {
				log.Error(err, "unable to refresh upgrade config")
			}
		case <-stopCh:
			log.Info("Stopping the upgradeConfigManager")
			break
		}
	}
}

func (s *upgradeConfigManager) RefreshUpgradeConfig(log logr.Logger) error {

	// If we don't have an internal cluster ID yet, fetch it from OCM
	if s.clusterID == "" {
		// TODO: hack! replace with using the actual OCM SDK, once the OCM SDK supports AccessToken auth.
		// Uncomment the block below for that.
		//cluster, err := getClusterFromOCMSDK(s.client, s.ocm.ClustersMgmt().V1().Clusters())
		clusterId, err := getClusterIdFromOCMAPI(s.client, s.httpClient)
		if err != nil {
			log.Error(err, "cannot obtain internal cluster ID")
			return err
		}
		s.clusterID = *clusterId
	}

	// Poll the upgrade service
	upgradeResponse, err := getClusterUpgradeStatus(s.clusterID, s.httpClient)
	if err != nil {
		log.Error(err, "cannot fetch next upgrade status")
		return err
	}

	// Apply UpgradeConfig changes
	err = applyUpgradeConfig(upgradeResponse, s.client)
	if err != nil {
		log.Error(err, "cannot apply upgrade config")
		return err
	}

	return nil
}

func getClusterUpgradeStatus(clusterId string, client *http.Client) (*clusterUpgrade, error) {

	// TODO: hack!! to test against a local mock upgrade policy service for now. replace with the line below.
	//url, err := url.Parse(OCM_API_URL)
	url, err := url.Parse("http://127.0.0.1:5000")
	if err != nil {
		log.Error(err, "OCM API URL unparsable.")
		return nil, fmt.Errorf("can't parse OCM API url: %v", err)
	}
	url.Path = path.Join(url.Path, CLUSTERS_V1_PATH, clusterId, UPGRADEPOLICIES_V1_PATH)

	request := &http.Request{
		Method: "GET",
		URL:    url,
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("can't query upgrade service: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received error code %v", response.StatusCode)
	}

	if response.Body != nil {
		defer response.Body.Close()
	}

	var upgradeResponse clusterUpgradeList
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&upgradeResponse)
	if err != nil {
		log.Error(err, "unable to decode upgrade service response")
		return nil, err
	}
	if upgradeResponse.Size != 1 || len(upgradeResponse.Items) != 1 {
		return nil, fmt.Errorf("no items returned from upgrade service")
	}

	return &upgradeResponse.Items[0], nil
}

func applyUpgradeConfig(upgrade *clusterUpgrade, c client.Client) error {
	// Fetch the current UpgradeConfig instance, if it exists
	upgradeConfigs := &upgradev1alpha1.UpgradeConfigList{}
	err := c.List(context.TODO(), upgradeConfigs, &client.ListOptions{})
	if err != nil {
		return fmt.Errorf("unable to retrieve upgradeconfigs")
	}

	upgradeConfig := upgradev1alpha1.UpgradeConfig{}
	isNewUpgradeConfig := false
	if len(upgradeConfigs.Items) == 0 {
		log.Info("No existing UpgradeConfig exists, creating new")
		isNewUpgradeConfig = true
		upgradeConfig.ObjectMeta.Name = UPGRADECONFIG_CR_NAME
		upgradeConfig.ObjectMeta.Namespace = UPGRADECONFIG_CR_NAMESPACE
	} else {
		upgradeConfig = upgradeConfigs.Items[0]
		log.Info(fmt.Sprintf("Found existing UpgradeConfig %v, will update", upgradeConfig.Name))
	}

	// Is there an upgrade scheduled?
	if upgrade.NextRun == "" {
		log.Info("No next upgrade date found from upgrade service, will remove upgrade configuration")
		// Only need to delete the existing upgrade config if it exists
		if !isNewUpgradeConfig {
			err = c.Delete(context.TODO(), &upgradeConfig, &client.DeleteOptions{})
		}
		if err != nil {
			return fmt.Errorf("unable to delete existing upgrade config: %v", err)
		}
		return nil
	}

	// Proceed with adding/updating the upgrade config..

	// determine channel from channel group
	upgradeChannel, err := inferUpgradeChannelFromChannelGroup(upgrade.Version.ChannelGroup, upgrade.Version.Id)
	if err != nil {
		return fmt.Errorf("unable to determine channel from channel group '%v' and version '%v'", upgrade.Version.ChannelGroup, upgrade.Version.Id)
	}

	upgradeConfig.Spec.Desired.Channel = *upgradeChannel
	upgradeConfig.Spec.PDBForceDrainTimeout = int32(upgrade.NodeDrainGracePeriod.Value)
	upgradeConfig.Spec.UpgradeAt = upgrade.NextRun
	upgradeConfig.Spec.Type = upgradev1alpha1.UpgradeType(upgrade.UpgradeType)
	upgradeConfig.Spec.Desired.Version = upgrade.Version.Id

	if isNewUpgradeConfig {
		err = c.Create(context.TODO(), &upgradeConfig, &client.CreateOptions{})
	} else {
		err = c.Update(context.TODO(), &upgradeConfig, &client.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("unable to apply UpgradeConfig changes: %v", err)
	}

	return nil
}

func setupOCMConnection(c client.Client) (*sdk.Connection, error) {

	// Fetch the cluster AccessToken
	accessToken, err := util.GetAccessToken(c)
	if err != nil {
		return nil, fmt.Errorf("cannot setup OCM connection: %v", err)
	}
	accessTokenStr := fmt.Sprintf("%s:%s", accessToken.ClusterId, accessToken.PullSecret)

	// Create the connection, and remember to close it:
	connection, err := sdk.NewConnectionBuilder().
		URL(OCM_API_URL).
		Tokens(accessTokenStr).
		Build()

	return connection, err
}

func getClusterFromOCMSDK(kc client.Client, csc *cmv1.ClustersClient) (*cmv1.Cluster, error) {

	// fetch the clusterversion, which contains the internal ID
	cv := &configv1.ClusterVersion{}
	err := kc.Get(context.TODO(), types.NamespacedName{Name: "version"}, cv)
	if err != nil {
		return nil, fmt.Errorf("can't get clusterversion: %v", err)
	}
	externalID := cv.Spec.ClusterID

	// query the cluster service for the cluster
	query := fmt.Sprintf("external_id = '%s'", externalID)

	response, err := csc.List().Search(query).Page(1).Size(1).Send()
	if err != nil {
		return nil, fmt.Errorf("failed to locate cluster '%s': %v", externalID, err)
	}

	// return the cluster if it was found

	switch response.Total() {
	case 0:
		return nil, fmt.Errorf("there is no cluster with identifier or name '%s'", externalID)
	case 1:
		return response.Items().Slice()[0], nil
	default:
		return nil, fmt.Errorf("there are %d clusters with identifier or name '%s'", response.Total(), externalID)
	}
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

// TODO: To be superceded by properly using the OCM SDK when supporting AccessToken auth
// See: getClusterFromOCMSDK
func getClusterIdFromOCMAPI(kc client.Client, client *http.Client) (*string, error) {

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

	url, err := url.Parse(OCM_API_URL)
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
		return nil, fmt.Errorf("can't query upgrade service: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received error code %v", response.StatusCode)
	}

	if response.Body != nil {
		defer response.Body.Close()
	}

	var listResponse clusterList
	decoder := json.NewDecoder(response.Body)
	err = decoder.Decode(&listResponse)
	if err != nil {
		log.Error(err, "unable to decode upgrade service response")
		return nil, err
	}
	if listResponse.Size != 1 || len(listResponse.Items) != 1 {
		return nil, fmt.Errorf("no items returned from upgrade service")
	}

	return &listResponse.Items[0].Id, nil
}

package ocmprovider

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/blang/semver"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/ocm"
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

// New returns a new ocmProvider
func New(client client.Client, ocmBaseUrl *url.URL) (*ocmProvider, error) {

	ocmClient, err := ocm.NewBuilder().New(client, ocmBaseUrl)
	if err != nil {
		return nil, err
	}
	return &ocmProvider{
		client:    client,
		ocmClient: ocmClient,
	}, nil
}

type ocmProvider struct {
	// Cluster k8s client
	client client.Client
	// OCM client
	ocmClient ocm.OcmClient
}

// Returns an indication of if the upgrade config had changed during the refresh,
// and indication of error if one occurs.
func (s *ocmProvider) Get() ([]upgradev1alpha1.UpgradeConfigSpec, error) {

	log.Info("Commencing sync with OCM Spec provider")

	cluster, err := s.ocmClient.GetCluster()
	if err != nil {
		log.Error(err, "cannot obtain internal cluster ID")
		// Pass the error up the chain if the cluster ID couldn't be found
		if err == ocm.ErrClusterIdNotFound {
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
	upgradePolicies, err := s.ocmClient.GetClusterUpgradePolicies(cluster.Id)
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

		policyState, err := s.ocmClient.GetClusterUpgradePolicyState(nextOccurringUpgradePolicy.Id, cluster.Id)
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

// getNextOccurringUpgradePolicy returns the next occurring upgradepolicy from a list of upgrade
// policies, regardless of the schedule_type.
func getNextOccurringUpgradePolicy(uPs *ocm.UpgradePolicyList) (*ocm.UpgradePolicy, error) {
	var nextOccurringUpgradePolicy ocm.UpgradePolicy

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
func isActionableUpgradePolicy(up *ocm.UpgradePolicy, state *ocm.UpgradePolicyState) bool {

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
func buildUpgradeConfigSpecs(upgradePolicy *ocm.UpgradePolicy, cluster *ocm.ClusterInfo, c client.Client) ([]upgradev1alpha1.UpgradeConfigSpec, error) {

	upgradeConfigSpecs := make([]upgradev1alpha1.UpgradeConfigSpec, 0)

	// Set the capacityReservation to true if it is not explicit specified in OCM
	var capacityReservation bool
	if upgradePolicy.CapacityReservation != nil && !*upgradePolicy.CapacityReservation {
		capacityReservation = false
	} else {
		capacityReservation = true
	}

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
		CapacityReservation:  capacityReservation,
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

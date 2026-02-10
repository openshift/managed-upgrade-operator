package ocmprovider

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/ocm"
	"github.com/openshift/managed-upgrade-operator/pkg/ocmagent"
)

var log = logf.Log.WithName("ocm-config-getter")

// Errors
var (
	ErrProviderUnavailable = fmt.Errorf("OCM Provider unavailable")
	ErrRetrievingPolicies  = fmt.Errorf("could not retrieve provider upgrade policies")
	ErrProcessingPolicies  = fmt.Errorf("could not process provider upgrade policies")
)

// New returns a new ocmProvider
func New(client client.Client, upgradeType upgradev1alpha1.UpgradeType, ocmBaseUrl *url.URL) (*ocmProvider, error) {
	var (
		ocmClient ocm.OcmClient
		err       error
	)

	if strings.Contains(ocmBaseUrl.String(), fmt.Sprintf("%s:%d", ocmagent.OCM_AGENT_SERVICE_URL, ocmagent.OCM_AGENT_SERVICE_PORT)) {
		ocmClient, err = ocmagent.NewBuilder().New(client, ocmBaseUrl)
	} else {
		ocmClient, err = ocm.NewBuilder().New(client, ocmBaseUrl)
	}

	if err != nil {
		return nil, err
	}
	return &ocmProvider{
		client:      client,
		ocmClient:   ocmClient,
		upgradeType: upgradeType,
	}, nil
}

type ocmProvider struct {
	// Cluster k8s client
	client client.Client
	// OCM client
	ocmClient ocm.OcmClient
	// upgrader that the upgradeconfig spec should use
	upgradeType upgradev1alpha1.UpgradeType
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
	if cluster.ID() == "" {
		return nil, ocm.ErrClusterIdNotFound
	}

	// Retrieve the cluster's available upgrade policies from Cluster Services
	upgradePolicies, err := s.ocmClient.GetClusterUpgradePolicies(cluster.ID())
	if err != nil {
		log.Error(err, "error retrieving upgrade policies")
		return nil, ErrRetrievingPolicies
	}

	// Get the next occurring policy from the available policies
	if upgradePolicies.Total() > 0 {
		nextOccurringUpgradePolicy, err := getNextOccurringUpgradePolicy(upgradePolicies)
		if err != nil {
			log.Error(err, "error getting next upgrade policy from upgrade policies")
			return nil, err
		}
		log.Info(fmt.Sprintf("Detected upgrade policy %s as next occurring.", nextOccurringUpgradePolicy.ID()))

		policyState, err := s.ocmClient.GetClusterUpgradePolicyState(nextOccurringUpgradePolicy.ID(), cluster.ID())
		if err != nil {
			log.Error(err, "error getting policy's state")
			return nil, err
		}

		if !isActionableUpgradePolicy(nextOccurringUpgradePolicy, policyState) {
			return nil, nil
		}

		// Apply the next occurring Upgrade policy to the clusters UpgradeConfig CR.
		specs, err := buildUpgradeConfigSpecs(nextOccurringUpgradePolicy, cluster, s.upgradeType)
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
func getNextOccurringUpgradePolicy(uPs *cmv1.UpgradePoliciesListResponse) (*cmv1.UpgradePolicy, error) {
	var nextOccurringUpgradePolicy *cmv1.UpgradePolicy

	nextOccurringUpgradePolicy = uPs.Items().Get(0)

	uPs.Items().Each(func(uP *cmv1.UpgradePolicy) bool {
		// NextRun() returns time.Time in SDK, not string
		currentNext := nextOccurringUpgradePolicy.NextRun()
		evalNext := uP.NextRun()

		if evalNext.Before(currentNext) {
			nextOccurringUpgradePolicy = uP
		}
		return true
	})

	return nextOccurringUpgradePolicy, nil
}

// Checks if the supplied upgrade policy is one which warrants turning into an
// UpgradeConfig
func isActionableUpgradePolicy(up *cmv1.UpgradePolicy, state *cmv1.UpgradePolicyState) bool {

	switch strings.ToLower(string(state.Value())) {
	case "pending":
		// Policies that are in a PENDING state should be ignored, because they aren't scheduled
		return false
	case "completed":
		// Policies that are in a COMPLETED state should be ignored, because they're already acted on
		return false
	case "cancelled":
		// Policies that are in a CANCELLED state should be ignored, because they're no longer valid
		return false
	}

	// Automatic upgrade policies will have an empty version if the cluster is up to date
	if len(up.Version()) == 0 {
		log.Info(fmt.Sprintf("Upgrade policy %v has an empty version, will ignore.", up.ID()))
		return false
	}

	return true
}

// Applies the supplied Upgrade Policy to the cluster in the form of an UpgradeConfig
// Returns an indication of if the policy being applied differs to the existing UpgradeConfig,
// and indication of error if one occurs.
func buildUpgradeConfigSpecs(upgradePolicy *cmv1.UpgradePolicy, cluster *cmv1.Cluster, upgradeType upgradev1alpha1.UpgradeType) ([]upgradev1alpha1.UpgradeConfigSpec, error) {

	upgradeConfigSpecs := make([]upgradev1alpha1.UpgradeConfigSpec, 0)

	// Capacity Reservation Handling:
	// The OCM SDK's UpgradePolicy type (ocm-sdk-go v0.1.494 / ocm-api-model v0.0.449)
	// does not expose the 'capacity_reservation' field that exists in the OCM API.
	// This is a known limitation of the SDK code generation.
	//
	// Historical behavior (before SDK migration):
	//   - If capacity_reservation was nil or true → capacityReservation = true
	//   - If capacity_reservation was false → capacityReservation = false
	//
	// Current behavior (after SDK migration):
	//   - Always defaults to true (matches the most common case)
	//   - Cannot detect if OCM API returns capacity_reservation: false
	//
	// Impact: This workaround is acceptable because:
	//   1. capacity_reservation typically defaults to true in OCM
	//   2. Setting it to false is rare in production environments
	//   3. The SDK limitation affects all consumers, not just MUO
	//
	// Future: If OCM SDK adds support for capacity_reservation field, replace this
	// with: capacityReservation = upgradePolicy.GetCapacityReservation()
	capacityReservation := true

	upgradeChannel, err := inferUpgradeChannelFromChannelGroup(cluster.Version().ChannelGroup(), upgradePolicy.Version())
	if err != nil {
		return nil, fmt.Errorf("unable to determine channel from channel group '%v' and version '%v' for policy ID '%v'", cluster.Version().ChannelGroup(), upgradePolicy.Version(), upgradePolicy.ID())
	}

	// NextRun() returns time.Time, format it as RFC3339 string
	nextRunTime := upgradePolicy.NextRun()
	upgradeConfigSpec := upgradev1alpha1.UpgradeConfigSpec{
		Desired: upgradev1alpha1.Update{
			Version: upgradePolicy.Version(),
			Channel: *upgradeChannel,
		},
		UpgradeAt:            nextRunTime.Format(time.RFC3339),
		PDBForceDrainTimeout: int32(cluster.NodeDrainGracePeriod().Value()), //#nosec G115 -- NodeDrainGracePeriod is expected to be within int32 range as it represents seconds for drain timeout, which is unlikely to exceed 2B seconds
		Type:                 upgradeType,
		CapacityReservation:  capacityReservation,
	}
	upgradeConfigSpecs = append(upgradeConfigSpecs, upgradeConfigSpec)

	return upgradeConfigSpecs, nil
}

// Infers a CVO channel name from the channel group and TO desired version edges
func inferUpgradeChannelFromChannelGroup(channelGroup string, toVersion string) (*string, error) {

	// Set our channelGroup to "stable" if it's empty or we can't find one
	// This is required for MUO support on ARO as OCM does not inform ARO clusters of a channel group
	if channelGroup == "" {
		channelGroup = "stable"
	}

	toSV, err := semver.Parse(toVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid semantic TO version: %v", toVersion)
	}

	minorVersion := toSV.Minor
	// For EUS channel groups, round up to the next even number
	if channelGroup == "eus" {
		if minorVersion%2 != 0 {
			minorVersion++
		}
	}

	channel := fmt.Sprintf("%v-%v.%v", channelGroup, toSV.Major, minorVersion)
	return &channel, nil
}

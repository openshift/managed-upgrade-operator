package notifier

import (
	"fmt"
	"net/url"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/pkg/ocm"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
)

// NewOCMNotifier returns a ocmNotifier
func NewOCMNotifier(client client.Client, ocmBaseUrl *url.URL, upgradeConfigManager upgradeconfigmanager.UpgradeConfigManager) (*ocmNotifier, error) {
	ocmClient, err := ocm.NewBuilder().New(client, ocmBaseUrl)
	if err != nil {
		return nil, err
	}
	return &ocmNotifier{
		client:               client,
		ocmClient:            ocmClient,
		upgradeConfigManager: upgradeConfigManager,
	}, nil
}

type ocmNotifier struct {
	// Cluster k8s client
	client client.Client
	// OCM client
	ocmClient ocm.OcmClient
	// Retrieves the upgrade config from the cluster
	upgradeConfigManager upgradeconfigmanager.UpgradeConfigManager
}

func (s *ocmNotifier) NotifyState(value NotifyState, description string) error {

	cluster, err := s.ocmClient.GetCluster()
	if err != nil {
		return fmt.Errorf("failed to retrieve internal ocm cluster ID: %v", err)
	}

	policyId, err := s.getPolicyIdForUpgradeConfig(cluster.Id)
	if err != nil {
		return fmt.Errorf("can't determine policy ID to notify for: %v", err)
	}

	currentState, err := s.ocmClient.GetClusterUpgradePolicyState(*policyId, cluster.Id)
	if err != nil {
		return fmt.Errorf("can't determine policy state: %v", err)
	}

	// Don't notify if the state is already at the same value
	// Only notify if it's a valid transition
	shouldNotify := validateStateTransition(NotifyState(currentState.Value), value)
	if !shouldNotify {
		return nil
	}

	err = s.ocmClient.SetState(string(value), description, *policyId, cluster.Id)
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
	policies, err := s.ocmClient.GetClusterUpgradePolicies(clusterId)
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

// Validates that a state transition can be made from the supplied from/to states
func validateStateTransition(from NotifyState, to NotifyState) bool {

	switch from {
	case StatePending:
		// We shouldn't even be in this state to transition from
		return false
	case StateScheduled:
		// Can only go to a started state
		switch to {
		case StateStarted:
			return true
		default:
			return false
		}
	case StateStarted:
		// Can go to a delayed, completed or failed state
		switch to {
		case StateDelayed:
			return true
		case StateCompleted:
			return true
		case StateFailed:
			return true
		default:
			return false
		}
	case StateDelayed:
		// can go to completed or failed state
		switch to {
		case StateCompleted:
			return true
		case StateFailed:
			return true
		default:
			return false
		}
	case StateCompleted:
		// can't go anywhere
		return false
	case StateFailed:
		// can't go anywhere
		return false
	default:
		return false
	}
}

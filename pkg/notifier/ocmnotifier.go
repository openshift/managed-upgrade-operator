package notifier

import (
	"fmt"
	"net/url"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/pkg/ocm"
	"github.com/openshift/managed-upgrade-operator/pkg/ocmagent"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
)

// NewOCMNotifier returns a ocmNotifier
func NewOCMNotifier(client client.Client, ocmBaseUrl *url.URL, upgradeConfigManager upgradeconfigmanager.UpgradeConfigManager) (*ocmNotifier, error) {
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
	return &ocmNotifier{
		client:               client,
		ocmClient:            ocmClient,
		upgradeConfigManager: upgradeConfigManager,
	}, nil
}

type OcmState string

const (
	OcmStatePending   OcmState = "pending"
	OcmStateStarted   OcmState = "started"
	OcmStateDelayed   OcmState = "delayed"
	OcmStateFailed    OcmState = "failed"
	OcmStateCompleted OcmState = "completed"
	OcmStateCancelled OcmState = "cancelled"
	OcmStateScheduled OcmState = "scheduled"
)

var stateMap = map[MuoState]OcmState{
	MuoStatePending:      OcmStatePending,
	MuoStateCancelled:    OcmStateCancelled,
	MuoStateStarted:      OcmStateStarted,
	MuoStateCompleted:    OcmStateCompleted,
	MuoStateDelayed:      OcmStateDelayed,
	MuoStateFailed:       OcmStateFailed,
	MuoStateScheduled:    OcmStateScheduled,
	MuoStateSkipped:      OcmStateDelayed,
	MuoStateScaleSkipped: OcmStateDelayed,
	MuoStateHealthCheck:  OcmStateDelayed,
}

var (
	// ServiceLogStateControlPlaneStarted defines the summary for control plane upgrade starting servicelog
	ServiceLogStateControlPlaneStarted = ServiceLogState{Summary: "Cluster is starting with control plane upgrade"}
	// ServiceLogStateControlPlaneFinished defines the summary for control plane upgrade finished servicelog
	ServiceLogStateControlPlaneFinished = ServiceLogState{Summary: "Cluster has finished control plane upgrade"}
	// ServiceLogStateWorkerPlaneFinished defines the summary for worker plane upgrade finished servicelog
	ServiceLogStateWorkerPlaneFinished = ServiceLogState{Summary: "Cluster has finished with worker plane upgrade"}
)

// ServiceLogState type defines the ServiceLog metadata
type ServiceLogState ocm.ServiceLog

var serviceLogMap = map[MuoState]ServiceLogState{
	MuoStateControlPlaneUpgradeStartedSL:  ServiceLogStateControlPlaneStarted,
	MuoStateControlPlaneUpgradeFinishedSL: ServiceLogStateControlPlaneFinished,
	MuoStateWorkerPlaneUpgradeFinishedSL:  ServiceLogStateWorkerPlaneFinished,
}

type ocmNotifier struct {
	// Cluster k8s client
	client client.Client
	// OCM client
	ocmClient ocm.OcmClient
	// Retrieves the upgrade config from the cluster
	upgradeConfigManager upgradeconfigmanager.UpgradeConfigManager
}

func (s *ocmNotifier) NotifyState(state MuoState, description string) error {

	cluster, err := s.ocmClient.GetCluster()
	if err != nil {
		return fmt.Errorf("failed to retrieve internal ocm cluster ID: %v", err)
	}

	if strings.HasSuffix(toString(state), "SL") {
		slState, ok := mapSLState(state, serviceLogMap)
		if !ok {
			return fmt.Errorf("failed to map the servicelog state for MUO state %s", state)
		}
		s.ocmClient.PostServiceLog((*ocm.ServiceLog)(&slState), description)
	}

	policyId, err := s.getPolicyIdForUpgradeConfig(cluster.Id)
	if err != nil {
		return fmt.Errorf("can't determine policy ID to notify for: %v", err)
	}

	currentState, err := s.ocmClient.GetClusterUpgradePolicyState(*policyId, cluster.Id)
	if err != nil {
		return fmt.Errorf("can't determine policy state: %v", err)
	}

	var muoCurrent MuoState
	// Return the MuoState from the current OcmState, determine if MUO is "skipped" or "delayed" it is OCM "deleyed"
	if OcmState(currentState.Value) == OcmStateDelayed {
		if strings.Contains(currentState.Description, "retry") {
			muoCurrent = MuoStateDelayed
		} else {
			muoCurrent = MuoStateSkipped
		}
	} else {
		mstate, ok := mapState(OcmState(currentState.Value), stateMap)
		if !ok {
			return fmt.Errorf("failed to convert OCM state")
		}
		muoCurrent = mstate
	}

	// Don't notify if the state is already at the same value
	// Only notify if it's a valid transition
	shouldNotify := validateStateTransition(muoCurrent, state)
	if !shouldNotify {
		return nil
	}

	err = s.ocmClient.SetState(string(stateMap[state]), description, *policyId, cluster.Id)
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
func validateStateTransition(from MuoState, to MuoState) bool {

	switch from {
	case MuoStatePending:
		// We shouldn't even be in this state to transition from
		return false
	case MuoStateScheduled:
		// Can only go to started state
		switch to {
		case MuoStateStarted:
			return true
		default:
			return false
		}

	case MuoStateStarted:
		// Can go to a scale skipped, healthCheck, delayed, completed or failed state
		switch to {
		case MuoStateScaleSkipped:
			return true
		case MuoStateDelayed:
			return true
		case MuoStateCompleted:
			return true
		case MuoStateFailed:
			return true
		case MuoStateHealthCheck:
			return true
		default:
			return false
		}

	case MuoStateScaleSkipped:
		// can go to skipped, delayed, completed or failed state
		switch to {
		case MuoStateDelayed:
			return true
		case MuoStateFailed:
			return true
		case MuoStateSkipped:
			return true
		case MuoStateCompleted:
			return true
		default:
			return false
		}

	case MuoStateDelayed:
		// can go to completed or failed or skipped state
		switch to {
		case MuoStateCompleted:
			return true
		case MuoStateFailed:
			return true
		case MuoStateSkipped:
			return true
		default:
			return false
		}

	case MuoStateSkipped:
		// can go to completed or failed state
		switch to {
		case MuoStateCompleted:
			return true
		case MuoStateFailed:
			return true
		default:
			return false
		}

	case MuoStateCompleted:
		// can't go anywhere
		return false
	case MuoStateFailed:
		// can't go anywhere
		return false
	default:
		return false
	}
}

// return the MuoState from the given OcmState
func mapState(os OcmState, dict map[MuoState]OcmState) (ms MuoState, ok bool) {
	for k, v := range dict {
		if v == os {
			ms = k
			ok = true
			return ms, ok
		}
	}
	return "", false
}

func toString(s MuoState) string {
	return string(s)
}

// return the ServiceLogState from the given MuoState
func mapSLState(ms MuoState, dict map[MuoState]ServiceLogState) (ss ServiceLogState, ok bool) {
	for k, v := range dict {
		if k == ms {
			ss = v
			ok = true
			return ss, ok
		}
	}
	return ServiceLogState{}, false
}

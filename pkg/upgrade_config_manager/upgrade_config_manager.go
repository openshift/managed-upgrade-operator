package upgrade_config_manager

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/openshift/managed-upgrade-operator/util"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"github.com/openshift-online/ocm-sdk-go"
)

const (
	OCM_API_URL = ""
)

var log = logf.Log.WithName("managed-upgrade-operator")

// TotalAccountWatcher global var for TotalAccountWatcher
var UpgradeConfigManager *upgradeConfigManager

type upgradeConfigManager struct {
	watchInterval time.Duration
	client        client.Client
}

// Initialize creates a global instance of the UpgradeConfigManager
func Initialize(client client.Client, watchInterval time.Duration) {
	log.Info("Initializing the upgradeConfigManager")

	// Set up the OCM client here
	_, err := setupOCMConnection(client)
	if err != nil {
		log.Error(err, "Failed to set up OCM connection")
		return
	}

	UpgradeConfigManager = NewUpgradeConfigManager(client, watchInterval)
	UpgradeConfigManager.RefreshUpgradeConfig(log)
}

// NewTotalAccountWatcher returns a new instance of the TotalAccountWatcher interface
func NewUpgradeConfigManager(
	client client.Client,
	watchInterval time.Duration,
) *upgradeConfigManager {
	return &upgradeConfigManager{
		watchInterval: watchInterval,
		client:        client,
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
	return nil
}

func setupOCMConnection(c client.Client) (*sdk.Connection, error) {

	// Fetch the cluster AccessToken
	accessToken, err := util.GetAccessToken(c)
	if err != nil {
		return nil, fmt.Errorf("cannot setup OCM connection: %v", err)
	}

	// Create the connection, and remember to close it:
	connection, err := sdk.NewConnectionBuilder().
		URL(OCM_API_URL).
		Tokens(*accessToken).
		Build()

	return connection, err
}

package upgraders

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	ac "github.com/openshift/managed-upgrade-operator/pkg/availabilitychecks"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/drain"
	"github.com/openshift/managed-upgrade-operator/pkg/eventmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	"github.com/openshift/managed-upgrade-operator/pkg/maintenance"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/scaler"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradesteps"
)

type clusterUpgrader struct {
	// Ordered list of steps to carry out a cluster upgrade
	steps []upgradesteps.UpgradeStep

	// Kube client used for interaction with cluster API
	client client.Client

	// Metrics client used for interaction with Prometheus
	metrics metrics.Metrics

	// ClusterVersion client used for interactions with the cluster's
	// clusterversion resource
	cvClient cv.ClusterVersion

	// EventManager client used for publishing upgrade events to a recipient
	notifier eventmanager.EventManager

	// Scaler used for performing upgrade-related capacity scaling
	scaler scaler.Scaler

	// External-availability checkers to satisfy pre-upgrade requirements
	availabilityCheckers ac.AvailabilityCheckers

	// UpgradeConfig that defines the upgrade being carried out
	upgradeConfig *upgradev1alpha1.UpgradeConfig

	// Builder of node drain strategies used to progress blocked drains
	drainstrategyBuilder drain.NodeDrainStrategyBuilder

	// Client used to manage maintenance windows during the upgrade
	maintenance maintenance.Maintenance

	// Client used to observe the state of machines in the cluster
	machinery machinery.Machinery

	// Model of the cluster upgrader's ConfigMap configuration
	config *upgraderConfig
}

// runSteps runs the upgrader's upgrade steps and returns the last-executed
// upgrade phase and any associated error
func (c *clusterUpgrader) runSteps(ctx context.Context, logger logr.Logger, s []upgradesteps.UpgradeStep) (upgradev1alpha1.UpgradePhase, error) {
	phase, err := upgradesteps.Run(ctx, c.upgradeConfig, logger, s)
	return phase, err
}

// UpgradeCluster performs the upgrade of the cluster and returns an indication of the
// last-executed upgrade phase and any error associated with the phase execution.
func (c *clusterUpgrader) UpgradeCluster(ctx context.Context, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (upgradev1alpha1.UpgradePhase, error) {
	c.upgradeConfig = upgradeConfig
	return c.runSteps(ctx, logger, c.steps)
}

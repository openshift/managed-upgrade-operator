package upgraders

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/notifier"
)

// PreUpgradeHealthCheck performs cluster healthy check
func (c *clusterUpgrader) PreUpgradeHealthCheck(ctx context.Context, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := c.cvClient.HasUpgradeCommenced(c.upgradeConfig)
	if err != nil {
		return false, err
	}
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("Skipping upgrade step %s", upgradev1alpha1.UpgradePreHealthCheck))
		return true, nil
	}

	version := getCurrentVersion(c.cvClient, logger)

	// Based on the "PreHealthCheck" featuregate, we invoke the legacy healthchecks for clusteroperator and critical alerts
	// which will not be tied to the notifications but only log the error and set metric.
	if !c.config.IsFeatureEnabled(string(upgradev1alpha1.PreHealthCheckFeatureGate)) {
		ok, err := CriticalAlerts(c.metrics, c.config, c.upgradeConfig, logger, version)
		if err != nil || !ok {
			return false, err
		}

		ok, err = ClusterOperators(c.metrics, c.cvClient, c.upgradeConfig, logger, version)
		if err != nil || !ok {
			return false, err
		}
	}

	// We invoke and handle the additional healthchecks accordingly with notifications enabled (or disabled via it's own featuregate)
	if len(c.config.FeatureGate.Enabled) > 0 && c.config.IsFeatureEnabled(string(upgradev1alpha1.PreHealthCheckFeatureGate)) {

		healthCheckFailed := []string{}
		history := c.upgradeConfig.Status.History.GetHistory(c.upgradeConfig.Spec.Desired.Version)
		state := string(history.Phase)

		ok, err := CriticalAlerts(c.metrics, c.config, c.upgradeConfig, logger, version)
		if err != nil || !ok {
			logger.Info("upgrade may delay due to firing critical alerts")
			healthCheckFailed = append(healthCheckFailed, "CriticalAlertsHealthcheckFailed")
		}

		ok, err = ClusterOperators(c.metrics, c.cvClient, c.upgradeConfig, logger, version)
		if err != nil || !ok {
			logger.Info("upgrade may delay due to cluster operators not ready")
			healthCheckFailed = append(healthCheckFailed, "ClusterOperatorsHealthcheckFailed")
		}

		if c.upgradeConfig.Spec.CapacityReservation {
			ok, err := c.scaler.CanScale(c.client, logger)
			if !ok || err != nil {
				c.metrics.UpdateMetricHealthcheckFailed(c.upgradeConfig.Name, metrics.DefaultWorkerMachinepoolNotFound, version, state)
				healthCheckFailed = append(healthCheckFailed, "CapacityReservationHealthcheckFailed")
			} else {
				logger.Info("Prehealth check for CapacityReservation passed")
				c.metrics.UpdateMetricHealthcheckSucceeded(c.upgradeConfig.Name, metrics.DefaultWorkerMachinepoolNotFound, version, state)
			}
		}
		var nodes []string
		nodes, err = ManuallyCordonedNodes(c.metrics, c.machinery, c.client, c.upgradeConfig, logger, version)
		if err != nil || nodes != nil {
			logger.Info(fmt.Sprintf("upgrade may delay due to there are manually cordoned nodes: %s", err))
			nodeNames := strings.Join(nodes, ",")
			healthCheckFailed = append(healthCheckFailed, fmt.Sprintf("NodeUnschedulableHealthcheckFailed:(%s)", nodeNames))
		}

		nodes, err = NodeUnschedulableTaints(c.metrics, c.machinery, c.client, c.upgradeConfig, logger, version)
		if err != nil || nodes != nil {
			logger.Info(fmt.Sprintf("upgrade delayed due to there are unschedulable taints on nodes: %s", err))
			nodeNames := strings.Join(nodes, ",")
			healthCheckFailed = append(healthCheckFailed, fmt.Sprintf("NodeUnschedulableTaintHealthcheckFailed:(%s)", nodeNames))
		}

		// HealthCheckPDB
		ok, err = HealthCheckPDB(c.metrics, c.client, c.dvo, c.upgradeConfig, logger, version)
		if err != nil || !ok {
			logger.Info(fmt.Sprintf("upgrade delayed due PDB %s", err))
			healthCheckFailed = append(healthCheckFailed, "PDBHealthcheckFailed")
		}

		if len(healthCheckFailed) > 0 {
			result := strings.Join(healthCheckFailed, ",")
			logger.Info(fmt.Sprintf("Upgrade may delay due to following PreHealthCheck failure: %s", result))

			switch history := c.upgradeConfig.Status.History.GetHistory(c.upgradeConfig.Spec.Desired.Version); history.Phase {
			case upgradev1alpha1.UpgradePhaseNew:
				err := c.notifier.NotifyResult(notifier.MuoStatePreHealthCheckSL, result)
				if err != nil {
					return false, err
				}
			case upgradev1alpha1.UpgradePhaseUpgrading:
				err := c.notifier.NotifyResult(notifier.MuoStateHealthCheckSL, result)
				if err != nil {
					return false, err
				}

				// Return false if the healthCheckFailed slice contains "CriticalAlertsHealthcheckFailed" or "ClusterOperatorsHealthcheckFailed"
				for _, healthcheck := range healthCheckFailed {
					if healthcheck == "CriticalAlertsHealthcheckFailed" || healthcheck == "ClusterOperatorsHealthcheckFailed" {
						return false, nil
					}
				}
				return true, nil
			case " ":
				logger.Info(fmt.Sprintf("upgradeconfig history doesn't exist for version: %s", c.upgradeConfig.Spec.Desired.Version))
			}
			return false, nil
		}
	}
	return true, nil
}

// PostUpgradeHealthCheck performs cluster healthy check
func (c *clusterUpgrader) PostUpgradeHealthCheck(ctx context.Context, logger logr.Logger) (bool, error) {
	version := getCurrentVersion(c.cvClient, logger)
	ok, err := CriticalAlerts(c.metrics, c.config, c.upgradeConfig, logger, version)
	if err != nil || !ok {
		return false, err
	}

	ok, err = ClusterOperators(c.metrics, c.cvClient, c.upgradeConfig, logger, version)
	if err != nil || !ok {
		return false, err
	}

	return true, nil
}

func getCurrentVersion(cvClient cv.ClusterVersion, logger logr.Logger) string {

	clusterVersion, err := cvClient.GetClusterVersion()
	if err != nil {
		// GetVersion failed should not block the upgrade
		logger.Error(err, "Get cluster version failed")
		return "unknown"
	}

	version, err := cv.GetCurrentVersion(clusterVersion)
	if err != nil {
		// GetVersion failed should not block the upgrade
		logger.Error(err, "Get current cluster version failed")
		return "unknown"
	}
	return version
}

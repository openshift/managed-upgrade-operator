package upgraders

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
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

	// Based on the "PreHealthCheck" featuregate, we invoke the legacy healthchecks for clusteroperator and critical alerts
	// which will not be tied to the notifications but only log the error and set metric.
	if !c.config.IsFeatureEnabled(string(upgradev1alpha1.PreHealthCheckFeatureGate)) {
		ok, err := CriticalAlerts(c.metrics, c.config, c.upgradeConfig, logger)
		if err != nil || !ok {
			return false, err
		}

		ok, err = ClusterOperators(c.metrics, c.cvClient, c.upgradeConfig, logger)
		if err != nil || !ok {
			return false, err
		}
	}

	// We invoke and handle the additional healthchecks accordingly with notifications enabled (or disabled via it's own featuregate)
	if len(c.config.FeatureGate.Enabled) > 0 && c.config.IsFeatureEnabled(string(upgradev1alpha1.PreHealthCheckFeatureGate)) {

		healthCheckFailed := []string{}
		history := c.upgradeConfig.Status.History.GetHistory(c.upgradeConfig.Spec.Desired.Version)
		state := string(history.Phase)
		version := getCurrentVersion(c.upgradeConfig)

		ok, err := CriticalAlerts(c.metrics, c.config, c.upgradeConfig, logger)
		if err != nil || !ok {
			logger.Info("upgrade may delay due to firing critical alerts")
			healthCheckFailed = append(healthCheckFailed, "CriticalAlertsHealthcheckFailed")
		}

		ok, err = ClusterOperators(c.metrics, c.cvClient, c.upgradeConfig, logger)
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

		ok, err = ManuallyCordonedNodes(c.metrics, c.machinery, c.client, c.upgradeConfig, logger)
		if err != nil || !ok {
			logger.Info(fmt.Sprintf("upgrade may delay due to there are manually cordoned nodes: %s", err))
			healthCheckFailed = append(healthCheckFailed, "NodeUnschedulableHealthcheckFailed")
		}

		ok, err = NodeUnschedulableTaints(c.metrics, c.machinery, c.client, c.upgradeConfig, logger)
		if err != nil || !ok {
			logger.Info(fmt.Sprintf("upgrade delayed due to there are unschedulable taints on nodes: %s", err))
			healthCheckFailed = append(healthCheckFailed, "NodeUnschedulableTaintHealthcheckFailed")
		}

		// HealthCheckPDB
		ok, err = HealthCheckPDB(c.metrics, c.client, c.dvo, c.upgradeConfig, logger)
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
	ok, err := CriticalAlerts(c.metrics, c.config, c.upgradeConfig, logger)
	if err != nil || !ok {
		return false, err
	}

	ok, err = ClusterOperators(c.metrics, c.cvClient, c.upgradeConfig, logger)
	if err != nil || !ok {
		return false, err
	}

	return true, nil
}

func getCurrentVersion(ug *upgradev1alpha1.UpgradeConfig) string {
	history := ug.Status.History.GetHistory(ug.Spec.Desired.Version)
	state := history.Phase
	version := ""

	switch state {
	case upgradev1alpha1.UpgradePhaseNew,
		upgradev1alpha1.UpgradePhasePending,
		upgradev1alpha1.UpgradePhaseFailed,
		upgradev1alpha1.UpgradePhaseUpgrading:
		version = history.PrecedingVersion
	case upgradev1alpha1.UpgradePhaseUpgraded:
		version = ug.Spec.Desired.Version
	case upgradev1alpha1.UpgradePhaseUnknown:
		version = "unknown"
	default:
		version = "Unsupported"
	}

	return version
}

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

	healthCheckFailed := []string{}

	ok, err := CriticalAlerts(c.metrics, c.config, c.upgradeConfig, logger)
	if err != nil || !ok {
		logger.Info("upgrade may delay due to firing critical alerts")
		healthCheckFailed = append(healthCheckFailed, "CriticalAlertsHealthcheckFailed")
	} else {
		c.metrics.UpdateMetricHealthcheckSucceeded(c.upgradeConfig.Name, metrics.CriticalAlertsFiring)
		c.metrics.UpdateMetricHealthcheckSucceeded(c.upgradeConfig.Name, metrics.MetricsQueryFailed)
	}

	ok, err = ClusterOperators(c.metrics, c.cvClient, c.upgradeConfig, logger)
	if err != nil || !ok {
		logger.Info("upgrade may delay due to cluster operators not ready")
		healthCheckFailed = append(healthCheckFailed, "ClusterOperatorsHealthcheckFailed")
	} else {
		c.metrics.UpdateMetricHealthcheckSucceeded(c.upgradeConfig.Name, metrics.ClusterOperatorsDegraded)
		c.metrics.UpdateMetricHealthcheckSucceeded(c.upgradeConfig.Name, metrics.ClusterOperatorsStatusFailed)
	}

	if c.upgradeConfig.Spec.CapacityReservation {
		ok, err := c.scaler.CanScale(c.client, logger)
		if !ok || err != nil {
			c.metrics.UpdateMetricHealthcheckFailed(c.upgradeConfig.Name, metrics.DefaultWorkerMachinepoolNotFound)
			healthCheckFailed = append(healthCheckFailed, "CapacityReservationHealthcheckFailed")
		} else {
			c.metrics.UpdateMetricHealthcheckSucceeded(c.upgradeConfig.Name, metrics.DefaultWorkerMachinepoolNotFound)
		}
	}

	ok, err = ManuallyCordonedNodes(c.metrics, c.machinery, c.client, c.upgradeConfig, logger)
	if err != nil || !ok {
		logger.Info(fmt.Sprintf("upgrade may delay due to there are manually cordoned nodes: %s", err))
		healthCheckFailed = append(healthCheckFailed, "NodeUnschedulableHealthcheckFailed")
	} else {
		c.metrics.UpdateMetricHealthcheckSucceeded(c.upgradeConfig.Name, metrics.ClusterNodesManuallyCordoned)
		c.metrics.UpdateMetricHealthcheckSucceeded(c.upgradeConfig.Name, metrics.ClusterNodeQueryFailed)
	}

	ok, err = NodeUnschedulableTaints(c.metrics, c.machinery, c.client, c.upgradeConfig, logger)
	if err != nil || !ok {
		logger.Info(fmt.Sprintf("upgrade delayed due to there are unschedulable taints on nodes: %s", err))
		healthCheckFailed = append(healthCheckFailed, "NodeUnschedulableTaintHealthcheckFailed")
	} else {
		c.metrics.UpdateMetricHealthcheckSucceeded(c.upgradeConfig.Name, metrics.ClusterNodesTaintedUnschedulable)
		c.metrics.UpdateMetricHealthcheckSucceeded(c.upgradeConfig.Name, metrics.ClusterNodeQueryFailed)
	}

	if len(healthCheckFailed) > 0 {
		result := strings.Join(healthCheckFailed, ",")
		logger.Info(fmt.Sprintf("Upgrade may delay due to following PreHealthCheck failure: %s", result))

		err := c.notifier.Notify(notifier.MuoStateHealthCheckSL)
		if err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// PostUpgradeHealthCheck performs cluster healthy check
func (c *clusterUpgrader) PostUpgradeHealthCheck(ctx context.Context, logger logr.Logger) (bool, error) {
	ok, err := CriticalAlerts(c.metrics, c.config, c.upgradeConfig, logger)
	if err != nil || !ok {
		return false, err
	} else {
		c.metrics.UpdateMetricHealthcheckSucceeded(c.upgradeConfig.Name, metrics.CriticalAlertsFiring)
		c.metrics.UpdateMetricHealthcheckSucceeded(c.upgradeConfig.Name, metrics.MetricsQueryFailed)
	}

	ok, err = ClusterOperators(c.metrics, c.cvClient, c.upgradeConfig, logger)
	if err != nil || !ok {
		return false, err
	} else {
		c.metrics.UpdateMetricHealthcheckSucceeded(c.upgradeConfig.Name, metrics.ClusterOperatorsDegraded)
		c.metrics.UpdateMetricHealthcheckSucceeded(c.upgradeConfig.Name, metrics.ClusterOperatorsStatusFailed)
	}
	return true, nil
}

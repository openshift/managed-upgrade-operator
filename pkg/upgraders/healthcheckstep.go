package upgraders

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
)

// PreUpgradeHealthCheck performs cluster healthy check
func (c *clusterUpgrader) PreUpgradeHealthCheck(ctx context.Context, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := c.cvClient.HasUpgradeCommenced(c.upgradeConfig)
	if err != nil {
		return false, err
	}
	desired := c.upgradeConfig.Spec.Desired
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("ClusterVersion is already set to Channel %s Version %s, skipping %s", desired.Channel, desired.Version, upgradev1alpha1.UpgradePreHealthCheck))
		return true, nil
	}

	ok, err := performClusterHealthCheck(c.metrics, c.cvClient, c.config, logger)
	if err != nil || !ok {
		c.metrics.UpdateMetricClusterCheckFailed(c.upgradeConfig.Name)
		return false, err
	}

	c.metrics.UpdateMetricClusterCheckSucceeded(c.upgradeConfig.Name)
	return true, nil
}

// PostUpgradeHealthCheck performs cluster healthy check
func (c *clusterUpgrader) PostUpgradeHealthCheck(ctx context.Context, logger logr.Logger) (bool, error) {
	ok, err := performClusterHealthCheck(c.metrics, c.cvClient, c.config, logger)
	if err != nil || !ok {
		c.metrics.UpdateMetricClusterCheckFailed(c.upgradeConfig.Name)
		return false, err
	}

	c.metrics.UpdateMetricClusterCheckSucceeded(c.upgradeConfig.Name)
	return true, nil
}

// check several things about the cluster and report problems
// * critical alerts
// * degraded operators (if there are critical alerts only)
func performClusterHealthCheck(metricsClient metrics.Metrics, cvClient cv.ClusterVersion, cfg *upgraderConfig, logger logr.Logger) (bool, error) {
	ic := cfg.HealthCheck.IgnoredCriticals
	icQuery := ""
	if len(ic) > 0 {
		icQuery = `,alertname!="` + strings.Join(ic, `",alertname!="`) + `"`
	}
	healthCheckQuery := `ALERTS{alertstate="firing",severity="critical",namespace=~"^openshift.*|^kube-.*|^default$",namespace!="openshift-customer-monitoring",namespace!="openshift-logging",namespace!="openshift-operators"` + icQuery + "}"
	alerts, err := metricsClient.Query(healthCheckQuery)
	if err != nil {
		return false, fmt.Errorf("unable to query critical alerts: %s", err)
	}

	alertCount := len(alerts.Data.Result)

	if alertCount > 0 {
		alert := []string{}
		uniqueAlerts := make(map[string]bool)

		for _, r := range alerts.Data.Result {
			a := r.Metric["alertname"]

			if uniqueAlerts[a] {
				continue
			}
			alert = append(alert, a)
			uniqueAlerts[a] = true
		}

		logger.Info(fmt.Sprintf("Critical alert(s) firing: %s. Cannot continue upgrade", strings.Join(alert, ", ")))
		return false, fmt.Errorf("critical alert(s) firing: %s", strings.Join(alert, ", "))
	}

	result, err := cvClient.HasDegradedOperators()
	if err != nil {
		return false, err
	}
	if len(result.Degraded) > 0 {
		logger.Info(fmt.Sprintf("Degraded operators: %s", strings.Join(result.Degraded, ", ")))
		// Send the metrics for the cluster check failed if we have degraded operators
		return false, fmt.Errorf("degraded operators: %s", strings.Join(result.Degraded, ", "))
	}

	return true, nil
}

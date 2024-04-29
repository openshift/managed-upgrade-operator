package upgraders

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
)

// CriticalAlerts function will check the list of alerts and namespaces to be ignored for healthcheck
// and filter the critical open firing alerts via the ALERTS metric.
func CriticalAlerts(metricsClient metrics.Metrics, cfg *upgraderConfig, ug *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	ic := cfg.HealthCheck.IgnoredCriticals
	icQuery := ""
	if len(ic) > 0 {
		icQuery = `,alertname!="` + strings.Join(ic, `",alertname!="`) + `"`
	}

	ignoredNamespace := cfg.HealthCheck.IgnoredNamespaces
	ignoredNamespaceQuery := ""
	if len(ignoredNamespace) > 0 {
		ignoredNamespaceQuery = `,namespace!="` + strings.Join(ignoredNamespace, `",namespace!="`) + `"`
	}

	healthCheckQuery := `ALERTS{alertstate="firing",severity="critical",namespace=~"^openshift.*|^kube-.*|^default$"` + ignoredNamespaceQuery + icQuery + "}"

	alerts, err := metricsClient.Query(healthCheckQuery)
	if err != nil {
		logger.Info(fmt.Sprintf("Unable to query metrics to check for open alerts"))
		metricsClient.UpdateMetricClusterCheckFailed(ug.Name)
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
		metricsClient.UpdateMetricClusterCheckFailed(ug.Name)

		return false, fmt.Errorf("critical alert(s) firing: %s", strings.Join(alert, ", "))
	}

	logger.Info(fmt.Sprintf("Prehealth check for critical alerts passed"))
	metricsClient.UpdateMetricClusterCheckSucceeded(ug.Name)
	return true, nil
}

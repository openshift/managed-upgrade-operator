package upgraders

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
)

// ClusterOperators function will check the degraded ClusterOperators and if there are any found then
// error is reported.
func ClusterOperators(metricsClient metrics.Metrics, cvClient cv.ClusterVersion, ug *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
	result, err := cvClient.HasDegradedOperators()
	if err != nil {
		logger.Info("Unable to fetch status of clusteroperators")
		metricsClient.UpdateMetricHealthcheckFailed(ug.Name, metrics.ClusterOperatorsStatusFailed)
		return false, err
	}
	if len(result.Degraded) > 0 {
		logger.Info(fmt.Sprintf("Degraded operators: %s", strings.Join(result.Degraded, ", ")))
		metricsClient.UpdateMetricHealthcheckFailed(ug.Name, metrics.ClusterOperatorsDegraded)
		return false, fmt.Errorf("degraded operators: %s", strings.Join(result.Degraded, ", "))
	}
	logger.Info("Prehealth check for clusteroperators passed")
	metricsClient.UpdateMetricHealthcheckSucceeded(ug.Name, metrics.ClusterOperatorsStatusFailed)
	metricsClient.UpdateMetricHealthcheckSucceeded(ug.Name, metrics.ClusterOperatorsDegraded)
	return true, nil
}

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
		return false, err
	}
	if len(result.Degraded) > 0 {
		logger.Info("Degraded operators: %s", strings.Join(result.Degraded, ", "))
		return false, fmt.Errorf("degraded operators: %s", strings.Join(result.Degraded, ", "))
	}
	logger.Info("Prehealth check for clusteroperators passed")
	return true, nil
}

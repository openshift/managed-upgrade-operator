package upgraders

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/dvo"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var namespaceException = []string{"openshift-logging", "openshift-redhat-marketplace", "openshift-operators", "openshift-customer-monitoring", "openshift-cnv", "openshift-route-monitoring-operator", "openshift-user-workload-monitoring", "openshift-pipelines"}

// HealthCheckPDB performs a health check on the PodDisruptionBudget (PDB) metrics.
// It returns true if the health check passes, false otherwise.
// It also returns an error if there was an issue performing the health check.
func HealthCheckPDB(metricsClient metrics.Metrics, c client.Client, dvo dvo.DvoClientBuilder, ug *upgradev1alpha1.UpgradeConfig, logger logr.Logger, version string) (bool, error) {

	// Get current cluster version and upgrade state info
	history := ug.Status.History.GetHistory(ug.Spec.Desired.Version)
	state := string(history.Phase)

	reason, err := checkPodDisruptionBudgets(c, logger)
	if err != nil {
		metricsClient.UpdateMetricHealthcheckFailed(ug.Name, reason, version, state)
		return false, err
	}

	reason, err = checkDvoMetrics(c, dvo, logger)
	if err != nil {
		metricsClient.UpdateMetricHealthcheckFailed(ug.Name, reason, version, state)
		return false, err
	}
	// Health check passed
	logger.Info("Prehealth check for PodDisruptionBudget passed")
	metricsClient.UpdateMetricHealthcheckSucceeded(ug.Name, metrics.ClusterInvalidPDB, version, state)

	return true, nil
}

func checkPodDisruptionBudgets(c client.Client, logger logr.Logger) (string, error) {
	// List all PodDisruptionBudgets
	pdbList := &policyv1.PodDisruptionBudgetList{}
	err := c.List(context.TODO(), pdbList)
	if err != nil {
		logger.Info("unable to list PodDisruptionBudgets/v1")
		return metrics.PDBQueryFailed, err
	}

	for _, pdb := range pdbList.Items {
		if !strings.HasPrefix(pdb.Namespace, "openshift-*") || checkNamespaceExistsInArray(namespaceException, pdb.Namespace) {

			maxResult, err := validateMaxUnavailable(pdb, logger)
			if !maxResult {
				return metrics.ClusterInvalidPDBConf, err
			}

			minResult, err := validateMinAvailable(pdb, logger)
			if !minResult {
				return metrics.ClusterInvalidPDBConf, err
			}
		}
	}

	return "", nil
}

func checkNamespaceExistsInArray(namespaceException []string, s string) bool {
	for _, namespace := range namespaceException {
		if namespace == s {
			return true
		}
	}
	return false
}

func checkDvoMetrics(c client.Client, dvo dvo.DvoClientBuilder, logger logr.Logger) (string, error) {
	// Create a new DVO client using the builder and the provided metrics client
	client, err := dvo.New(c)
	if err != nil {
		return metrics.DvoClientCreationFailed, err
	}

	// Get the PDB metrics
	dvoMetricsResult, err := client.GetMetrics()
	if err != nil {
		logger.Info("Error getting DVO metrics")
		return metrics.DvoMetricsQueryFailed, err
	}

	badPDBExists := false
	for _, metric := range dvoMetricsResult {
		if strings.Contains(string(metric), "deployment_validation_operator_pdb_min_available") || strings.Contains(string(metric), "deployment_validation_operator_pdb_max_available") {
			badPDBExists = true
			break
		}
	}
	if badPDBExists {
		return metrics.ClusterInvalidPDB, fmt.Errorf("found a PodDisruptionBudget with incorrect configurations")
	}

	return "", nil
}

// validateMaxUnavailable function will return false for failures if
// MaxUnavailable for a PDB is 0 or 0% if defined
func validateMaxUnavailable(p policyv1.PodDisruptionBudget, l logr.Logger) (bool, error) {
	if p.Spec.MaxUnavailable != nil {
		switch p.Spec.MaxUnavailable.Type {
		case intstr.Int:
			if p.Spec.MaxUnavailable.IntVal == 0 {
				l.Info(fmt.Sprintf("MaxUnavailable 0 found in PodDisruptionBudget: %s/%s\n", p.Namespace, p.Name))
				return false, fmt.Errorf("found a PodDisruptionBudget with MaxUnavailable set to 0")
			}
		case intstr.String:
			if p.Spec.MaxUnavailable.StrVal == "0%" {
				l.Info(fmt.Sprintf("MaxUnavailable 0%% found in PodDisruptionBudget: %s/%s\n", p.Namespace, p.Name))
				return false, fmt.Errorf("found a PodDisruptionBudget with MaxUnavailable set to 0%%")
			}
		}
	}
	return true, nil
}

// validateMinAvailable function will return false for failures if
// MinAvailable for a PDB is 100% if defined
func validateMinAvailable(p policyv1.PodDisruptionBudget, l logr.Logger) (bool, error) {
	if p.Spec.MinAvailable != nil {
		switch p.Spec.MinAvailable.Type {
		case intstr.String:
			if p.Spec.MinAvailable.StrVal == "100%" {
				l.Info(fmt.Sprintf("MinAvailable 100%% found in PodDisruptionBudget: %s/%s\n", p.Namespace, p.Name))
				return false, fmt.Errorf("found a PodDisruptionBudget with MinUnavailable set to 100%%")
			}
		}
	}
	return true, nil
}

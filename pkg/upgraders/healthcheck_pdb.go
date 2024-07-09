package upgraders

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/managed-upgrade-operator/pkg/dvo"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	policyv1 "k8s.io/api/policy/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HealthCheckPDB performs a health check on the PodDisruptionBudget (PDB) metrics.
// It returns true if the health check passes, false otherwise.
// It also returns an error if there was an issue performing the health check.
func HealthCheckPDB(metricsClient metrics.Metrics, c client.Client) (bool, error) {
	// List all PodDisruptionBudgets
	pdbList := &policyv1.PodDisruptionBudgetList{}
	err1 := c.List(context.TODO(), pdbList)
	if err1 != nil {
		fmt.Print("unable to list PodDisruptionBudgets/v1")
		return false, err1
	}

	for _, pdb := range pdbList.Items {
		if !strings.HasPrefix(pdb.Namespace, "openshift-*") {
			if pdb.Spec.MaxUnavailable != nil && pdb.Spec.MaxUnavailable.IntVal == 0 {
				//BAD pdb
				return false, fmt.Errorf("found a PodDisruptionBudget with MaxUnavailable set to 0")
			} else if pdb.Status.CurrentHealthy < pdb.Status.DesiredHealthy {
				//BAD pdb
				return false, fmt.Errorf("found a PodDisruptionBudget with CurrentHealthy less than DesiredHealthy")
			}
		}
	}

	// Create a new DVO client using the builder and the provided Kubernetes client
	builder := dvo.NewBuilder()
	client, err := builder.New(c)
	if err != nil {
		return false, err
	}

	// Get the PDB metrics
	dvoMetricsResult, err := client.GetMetrics()
	if err != nil {
		fmt.Println("Error getting metrics")
		return false, err
	}

	badPDBExists := false
	for _, metric := range dvoMetricsResult {
		if strings.Contains(string(metric), "deployment_validation_operator_pdb_min_available") || strings.Contains(string(metric), "deployment_validation_operator_pdb_max_available") {
			badPDBExists = true
			break
		}
	}
	if badPDBExists {
		return false, fmt.Errorf("found a PodDisruptionBudget with incorrect configurations")
	}

	// Health check passed
	return true, nil
}

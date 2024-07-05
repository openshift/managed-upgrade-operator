package upgraders

import (
	"fmt"
	"strconv"

	"github.com/openshift/managed-upgrade-operator/pkg/dvo"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HealthCheckPDB performs a health check on the PodDisruptionBudget (PDB) metrics.
// It returns true if the health check passes, false otherwise.
// It also returns an error if there was an issue performing the health check.
func HealthCheckPDB(metricsClient metrics.Metrics, c client.Client) (bool, error) {

	// Create a new DVO builder
	builder := dvo.NewBuilder()

	// Create a new DVO client using the builder and the provided Kubernetes client
	client, err := builder.New(c)
	if err != nil {
		return false, err
	}

	// Get the PDB metrics
	pdbresult, err := client.GetMetrics()
	if err != nil {
		fmt.Println("Error getting metrics")
		return false, err
	}

	// Calculate the number of PDBs with max unavailable pods set
	PDBMaxUnavailableRes := len(pdbresult.Data.Result)
	var disruptionsAllowed string
	if PDBMaxUnavailableRes > 0 {
		for _, r := range pdbresult.Data.Result {
			a := r.Metric["Pdb-max-unavailable"]
			disruptionsAllowed = a
		}
	}

	// Convert the disruptionsAllowed string to an integer
	disruptionsAllowedInt, _ := strconv.Atoi(disruptionsAllowed)

	// Check if disruptionsAllowedInt is zero
	if disruptionsAllowedInt == 0 {
		// failure case
		fmt.Println("disruptions allowed is zero")
		return false, nil
	}

	// Max Unavailable Pods Constraint
	maxUnavailablePods := disruptionsAllowedInt // Change this value to the desired max unavailable pods constraint

	// Check if maxUnavailablePods is less than zero
	if maxUnavailablePods < 0 {
		return false, fmt.Errorf("max unavailable pods constraint is less than zero")
	}

	// Health check passed
	return true, nil
}

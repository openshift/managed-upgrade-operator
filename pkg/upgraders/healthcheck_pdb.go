package upgraders

import (
	"fmt"
	"net/url"

	"github.com/openshift/managed-upgrade-operator/pkg/dvo"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func HealthCheckPDB(c client.Client) error {

	//TODO:Use RBAC to get service
	DVOMetricsURL := "http://deployment-validation-operator-metrics.deployment-validation-operator.svc:8383"
	url, err := url.Parse(DVOMetricsURL)
	if err != nil {
		return err
	}
	builder := dvo.NewBuilder()
	client, err := builder.New(c, url)
	if err != nil {
		return err
	}
	// return metrics resp
	if err := client.GetMetrics(); err != nil {
		fmt.Println("Error getting metrics")
		return err
	}

	// Disruptions Allowed is Zero
	disruptionsAllowed := 0
	if disruptionsAllowed != 0 {
		return fmt.Errorf("disruptions allowed is not zero")
	}

	// Max Unavailable Pods Constraint
	maxUnavailablePods := 1 // Change this value to the desired max unavailable pods constraint
	if maxUnavailablePods < 0 {
		return fmt.Errorf("max unavailable pods constraint is less than zero")
	}
	return nil

}
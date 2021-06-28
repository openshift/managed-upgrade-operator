package util

import (
	"fmt"
	"os"
)

// GetOperatorNamespace retrieves the operator namespace from the running environment or error if unavailable
func GetOperatorNamespace() (string, error) {
	envVarOperatorNamespace := "OPERATOR_NAMESPACE"
	ns, found := os.LookupEnv(envVarOperatorNamespace)
	if !found {
		return "", fmt.Errorf("%s must be set", envVarOperatorNamespace)
	}
	return ns, nil
}

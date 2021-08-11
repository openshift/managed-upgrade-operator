package config

import (
	"os"
	"time"

	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/util"
)

const (
	// OperatorName is the name of the operator
	OperatorName string = "managed-upgrade-operator"
	// OperatorNamespace is the namespace of the operator
	OperatorNamespace string = "openshift-managed-upgrade-operator"
	// SyncPeriodDefault reconciles a sync period for each controller
	SyncPeriodDefault = 5 * time.Minute
	// ConfigMapName is the name of the ConfigMap for the operator
	ConfigMapName string = OperatorName + "-config"
	// ConfigField is the name of field within the ConfigMap
	ConfigField string = "config.yaml"
	// EnvRoutes is used to determine if routes should be used during development
	EnvRoutes string = "ROUTES"
)

type CMTarget configmanager.Target

// NewCMTarget acts as a wrapper around configmanager.Target to enable
// MUO defaults that can be set outside of the configmanager pkg itself.
func (c *CMTarget) NewCMTarget() (configmanager.Target, error) {
	var err error
	if c.Name == "" {
		c.Name = ConfigMapName
	}

	if c.Namespace == "" {
		c.Namespace, err = util.GetOperatorNamespace()
	}

	if c.ConfigKey == "" {
		c.ConfigKey = ConfigField
	}
	return configmanager.Target{
		Name:      c.Name,
		Namespace: c.Namespace,
		ConfigKey: c.ConfigKey,
	}, err
}

func UseRoutes() bool {
	return os.Getenv(EnvRoutes) == "true"
}

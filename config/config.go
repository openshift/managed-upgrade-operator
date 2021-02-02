package config

import "time"

const (
	OperatorName string = "managed-upgrade-operator"
	OperatorNamespace string = "managed-upgrade-operator"
	// Default reconcile sync period for each controller
	SyncPeriodDefault = 5 * time.Minute
)

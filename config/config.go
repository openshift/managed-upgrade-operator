package config

import "time"

// TODO: put the name of the config in here
const (
	// OperatorName is the name of the operator
	OperatorName string = "managed-upgrade-operator"
	// OperatorNamespace is the namespace of the operator
	OperatorNamespace string = "managed-upgrade-operator"
	// SyncPeriodDefault reconciles a sync period for each controller
	SyncPeriodDefault = 5 * time.Minute
)

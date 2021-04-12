package config

import "time"

const (
	OperatorName      string = "managed-upgrade-operator"
	OperatorNamespace string = "managed-upgrade-operator"
	OperatorAcronym   string = "muo"
	// Default reconcile sync period for each controller
	SyncPeriodDefault = 5 * time.Minute
)

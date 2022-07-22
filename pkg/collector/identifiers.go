package collector

// generics for metric construction
const (
	MetricsNamespace   = "managed_upgrade"
	subSystemUpgrade   = "upgrade"
	subSystemCollector = "collector"
	subSystemCondition = "condition"
)

// keys for labels
const (
	keyPhase             = "phase"
	keyUpgradeConfigName = "upgradeconfig_name"
	keyVersion           = "version"
	keyDesiredVersion    = "desired_version"
	keyCondition         = "condition"
)

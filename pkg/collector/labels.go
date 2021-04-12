package collector

// generics for metric construction
const (
	subSystemCluster       = "cluster"
	subSystemUpgrade       = "upgrade"
	subSystemUpgradeConfig = "upgradeconfig"
	subSystemNotification  = "notification"
)

// keys for labels
const (
	keyPhase             = "phase"
	keyNodeName          = "node_name"
	keyUpgradeConfigName = "upgradeconfig_name"
	keyScaleEvent        = "scale_event"
	keyDimension         = "dimension"
	keyVersion           = "version"
	keyUpgrade           = "upgrade"

	// OSD only - used for notifications
	keyState = "state"

	nodeLabel  = "node_name"
	metricsTag = "upgradeoperator"
)

//TODO @dofinn: Should we review these states and make the CR.status.phase more
// granular to make these a 1:1 mapping?
// https://github.com/openshift/managed-upgrade-operator/blob/master/pkg/apis/upgrade/v1alpha1/upgradeconfig_types.go#L121-L133

// values for labels
const (
	valueControlPlaneCompleted = "control_plane_completed"
	valueControlPlaneStarted   = "control_plane_started"
	valueCompleted             = "completed"
	valuePending               = "pending"
	valueStarted               = "started"
	ValuePostUpgrade           = "post_upgrade"
	ValuePreUpgrade            = "pre_upgrade"
	valueUpgrading             = "upgrading"
	valueWorkersStarted        = "workers_started"
	valueWorkersCompleted      = "workers_completed"

	valueEvent = "event"
)

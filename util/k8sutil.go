package util

// ForceRunModeEnv indicates if the operator should be forced to run in either local
// or cluster mode (currently only used for local mode)
var ForceRunModeEnv = "OSDK_FORCE_RUN_MODE"

type RunModeType string

const (
	LocalRunMode   RunModeType = "local"
	ClusterRunMode RunModeType = "cluster"
)

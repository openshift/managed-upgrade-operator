package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpgradePreflightCheckSpec defines the target upgrade to check readiness for.
// It embeds UpgradeConfigSpec so that all UpgradeConfig fields (type, desired version,
// PDBForceDrainTimeout, etc.) are inherited automatically. Fields like upgradeAt and
// capacityReservation are present but unused by the preflight controller.
type UpgradePreflightCheckSpec struct {
	UpgradeConfigSpec `json:",inline"`

	// CheckTimeoutSeconds is the maximum number of seconds each individual check
	// may run before it is cancelled and recorded as State=Error.
	// Defaults to 30 seconds.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=30
	CheckTimeoutSeconds int32 `json:"checkTimeoutSeconds,omitempty"`

	// Checks is an optional allowlist of check names to run. When empty, all checks run.
	// Checks not in this list are recorded with State=Skipped.
	// Valid names: ClusterVersion, CriticalAlerts, ClusterOperators,
	//              ManuallyCordonedNodes, NodeUnschedulableTaints, PodDisruptionBudgets
	// +kubebuilder:validation:Optional
	Checks []string `json:"checks,omitempty"`
}

// CheckExecutionState describes whether a check was able to run.
// +kubebuilder:validation:Enum={"Completed","Error","Skipped"}
type CheckExecutionState string

const (
	// CheckCompleted means the check ran to completion; see Result for findings.
	CheckCompleted CheckExecutionState = "Completed"
	// CheckError means the check could not run due to an infrastructure or transient error.
	CheckError CheckExecutionState = "Error"
	// CheckSkipped means the check was excluded by the Checks allowlist in the spec.
	CheckSkipped CheckExecutionState = "Skipped"
)

// CheckExecution describes whether the check machinery ran successfully.
type CheckExecution struct {
	// State is the execution outcome: Completed, Error, or Skipped.
	State CheckExecutionState `json:"state"`
	// Error contains the reason a check could not run (State=Error only).
	// This is an infrastructure/transient failure, not a cluster health finding.
	// +kubebuilder:validation:Optional
	Error string `json:"error,omitempty"`
}

// CheckVerdict is the health verdict when a check completes successfully.
// +kubebuilder:validation:Enum={"Passed","Failed"}
type CheckVerdict string

const (
	CheckPassed CheckVerdict = "Passed"
	CheckFailed CheckVerdict = "Failed"
)

// CheckFinding describes what a completed check found about cluster health.
// Only present when CheckExecution.State=Completed.
type CheckFinding struct {
	// Verdict is Passed if the cluster is healthy for this check, Failed otherwise.
	Verdict CheckVerdict `json:"verdict"`
	// Message is a human-readable summary of the finding (present on Failed).
	// +kubebuilder:validation:Optional
	Message string `json:"message,omitempty"`
	// Details contains per-item findings (e.g. individual alert names, node names).
	// Structured for tooling; parse with jq/yq via .result.details[].
	// +kubebuilder:validation:Optional
	Details []string `json:"details,omitempty"`
}

// PreflightCheckResult holds the execution state and finding for a single preflight check.
type PreflightCheckResult struct {
	// Name identifies the check (e.g. "CriticalAlerts", "ClusterOperators").
	Name string `json:"name"`
	// Execution describes whether the check ran successfully.
	Execution CheckExecution `json:"execution"`
	// Result contains the health finding. Only present when Execution.State=Completed.
	// +kubebuilder:validation:Optional
	Result *CheckFinding `json:"result,omitempty"`
}

// UpgradePreflightCheckStatus reports the outcome of all checks.
type UpgradePreflightCheckStatus struct {
	// +kubebuilder:validation:Optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// +kubebuilder:validation:Optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
	// +kubebuilder:validation:Optional
	Checks []PreflightCheckResult `json:"checks,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=upgradepreflightchecks,scope=Namespaced,shortName=pfc
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".spec.desired.version"
// +kubebuilder:printcolumn:name="channel",type="string",JSONPath=".spec.desired.channel"
// +kubebuilder:printcolumn:name="type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="started",type="string",JSONPath=".status.startedAt"
// +kubebuilder:printcolumn:name="completed",type="string",JSONPath=".status.completedAt"

// UpgradePreflightCheck is the Schema for on-demand cluster upgrade readiness checks.
// Creating one runs pre-upgrade health checks without scheduling or starting an upgrade.
// Checks are currently platform-agnostic; type- and version-aware checks are planned.
type UpgradePreflightCheck struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UpgradePreflightCheckSpec   `json:"spec,omitempty"`
	Status UpgradePreflightCheckStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UpgradePreflightCheckList contains a list of UpgradePreflightCheck.
type UpgradePreflightCheckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UpgradePreflightCheck `json:"items"`
}

func init() {
	SchemeBuilder.Register(&UpgradePreflightCheck{}, &UpgradePreflightCheckList{})
}

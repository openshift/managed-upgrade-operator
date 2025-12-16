package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InsightType defines the type of AI insight to generate
type InsightType string

const (
	// InsightTypeReadiness analyzes pre-upgrade cluster readiness
	InsightTypeReadiness InsightType = "readiness"
	// InsightTypeEventClassification classifies and summarizes upgrade-related events
	InsightTypeEventClassification InsightType = "event-classification"
	// InsightTypePostUpgradeDebug analyzes post-upgrade issues and failures
	InsightTypePostUpgradeDebug InsightType = "post-upgrade-debug"
	// InsightTypeLogTailAnalysis analyzes recent pod logs for errors and patterns
	InsightTypeLogTailAnalysis InsightType = "log-tail-analysis"
)

// UpgradeInsightSpec defines the desired state of UpgradeInsight
type UpgradeInsightSpec struct {
	// Target references the UpgradeConfig to analyze
	Target TargetRef `json:"target"`

	// InsightTypes specifies which types of insights to generate
	// +kubebuilder:validation:MinItems=1
	InsightTypes []InsightType `json:"insightTypes"`

	// Options for insight generation
	// +optional
	Options InsightOptions `json:"options,omitempty"`

	// Disabled allows temporarily disabling insight generation
	// +optional
	Disabled bool `json:"disabled,omitempty"`
}

// TargetRef references the UpgradeConfig to analyze
type TargetRef struct {
	// API version of the referent
	APIVersion string `json:"apiVersion"`
	// Kind of the referent
	Kind string `json:"kind"`
	// Name of the referent
	Name string `json:"name"`
}

// InsightOptions configures the insight generation behavior
type InsightOptions struct {
	// MaxEvents limits the number of events to include in analysis
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000
	// +kubebuilder:default=200
	// +optional
	MaxEvents int `json:"maxEvents,omitempty"`

	// IncludeTimeWindowMinutes limits event collection to this time window
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1440
	// +kubebuilder:default=60
	// +optional
	IncludeTimeWindowMinutes int `json:"includeTimeWindowMinutes,omitempty"`

	// MaxLogLines limits the number of log lines to analyze per pod
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=1000
	// +kubebuilder:default=100
	// +optional
	MaxLogLines int `json:"maxLogLines,omitempty"`

	// TargetPods specifies which pods to collect logs from (label selector)
	// +optional
	TargetPods *metav1.LabelSelector `json:"targetPods,omitempty"`
}

// UpgradeInsightStatus defines the observed state of UpgradeInsight
type UpgradeInsightStatus struct {
	// ObservedAt is the timestamp when this insight was generated
	// +optional
	ObservedAt *metav1.Time `json:"observedAt,omitempty"`

	// ModelName is the name of the LLM model used
	// +optional
	ModelName string `json:"modelName,omitempty"`

	// Insights contains the AI-generated analysis
	// +optional
	Insights InsightResult `json:"insights,omitempty"`

	// Error contains any error message from insight generation
	// +optional
	Error string `json:"error,omitempty"`

	// Conditions represent the latest available observations of the insight's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// InsightResult contains all AI-generated insights
type InsightResult struct {
	// Readiness provides pre-upgrade readiness analysis
	// +optional
	Readiness *ReadinessInsight `json:"readiness,omitempty"`

	// EventClassification provides event analysis
	// +optional
	EventClassification *EventClassificationInsight `json:"eventClassification,omitempty"`

	// PostUpgrade provides post-upgrade debugging insights
	// +optional
	PostUpgrade *PostUpgradeInsight `json:"postUpgrade,omitempty"`

	// LogTailAnalysis provides log analysis insights
	// +optional
	LogTailAnalysis *LogTailInsight `json:"logTailAnalysis,omitempty"`
}

// ReadinessInsight contains pre-upgrade readiness analysis
type ReadinessInsight struct {
	// Summary provides a high-level readiness assessment
	Summary string `json:"summary"`

	// RiskScore is a 0-100 score indicating upgrade risk
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	RiskScore int `json:"riskScore"`

	// Blockers lists potential upgrade blockers
	// +optional
	Blockers []Blocker `json:"blockers,omitempty"`
}

// Blocker describes a potential upgrade blocker
type Blocker struct {
	// Resource identifies the blocking resource
	Resource string `json:"resource"`

	// Reason explains why this is a blocker
	Reason string `json:"reason"`

	// Severity indicates the blocker severity (high, medium, low)
	// +optional
	Severity string `json:"severity,omitempty"`
}

// EventClassificationInsight contains event analysis
type EventClassificationInsight struct {
	// NoiseCount is the number of events classified as noise
	NoiseCount int `json:"noiseCount"`

	// Concerning lists events that may indicate problems
	// +optional
	Concerning []ConcerningEvent `json:"concerning,omitempty"`
}

// ConcerningEvent describes an event that may indicate a problem
type ConcerningEvent struct {
	// Type is the event type or category
	Type string `json:"type"`

	// Explanation provides context for why this is concerning
	Explanation string `json:"explanation"`

	// Count is the number of similar events
	// +optional
	Count int `json:"count,omitempty"`
}

// PostUpgradeInsight contains post-upgrade analysis
type PostUpgradeInsight struct {
	// Issues lists identified post-upgrade issues
	// +optional
	Issues []Issue `json:"issues,omitempty"`

	// Recommendations provides actionable next steps
	// +optional
	Recommendations []string `json:"recommendations,omitempty"`
}

// Issue describes a post-upgrade issue
type Issue struct {
	// Component identifies the affected component
	Component string `json:"component"`

	// Description explains the issue
	Description string `json:"description"`

	// Impact describes the user-facing impact
	// +optional
	Impact string `json:"impact,omitempty"`
}

// LogTailInsight contains log analysis results
type LogTailInsight struct {
	// Summary provides an overview of log analysis
	Summary string `json:"summary"`

	// ErrorPatterns lists detected error patterns
	// +optional
	ErrorPatterns []LogPattern `json:"errorPatterns,omitempty"`

	// AnomalousLogs highlights unusual log entries
	// +optional
	AnomalousLogs []AnomalousLog `json:"anomalousLogs,omitempty"`
}

// LogPattern describes a pattern found in logs
type LogPattern struct {
	// Pattern is the detected pattern
	Pattern string `json:"pattern"`

	// Occurrences is how many times this pattern appeared
	Occurrences int `json:"occurrences"`

	// Explanation provides context for this pattern
	// +optional
	Explanation string `json:"explanation,omitempty"`
}

// AnomalousLog represents an unusual log entry
type AnomalousLog struct {
	// Pod is the source pod
	Pod string `json:"pod"`

	// Message is the log message
	Message string `json:"message"`

	// Reason explains why this is anomalous
	Reason string `json:"reason"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=upgradeinsights,scope=Namespaced,shortName=insight
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.target.name"
// +kubebuilder:printcolumn:name="Model",type="string",JSONPath=".status.modelName"
// +kubebuilder:printcolumn:name="ObservedAt",type="date",JSONPath=".status.observedAt"
// +kubebuilder:printcolumn:name="Error",type="string",JSONPath=".status.error"

// UpgradeInsight is the Schema for the upgradeinsights API
type UpgradeInsight struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UpgradeInsightSpec   `json:"spec,omitempty"`
	Status UpgradeInsightStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UpgradeInsightList contains a list of UpgradeInsight
type UpgradeInsightList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UpgradeInsight `json:"items"`
}

func init() {
	SchemeBuilder.Register(&UpgradeInsight{}, &UpgradeInsightList{})
}

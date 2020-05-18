package v1alpha1

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpgradeConfigSpec defines the desired state of UpgradeConfig and upgrade window and freeze window
type UpgradeConfigSpec struct {
	// Specify the desired OpenShift release
	Desired       Update        `json:"desired"`

	// This defines the 3rd party operator subscriptions upgrade
	// +kubebuilder:validation:Optional
	SubscriptionUpdates []SubscriptionUpdate `json:"subscriptionUpdates,omitempty"`
	UpgradeWindow UpgradeWindow `json:"upgradeWindow"`
	FreezeWindow  FreezeWindow  `json:"freezeWindow"`
}

// UpgradeConfigStatus defines the observed state of UpgradeConfig
type UpgradeConfigStatus struct {

	// +kubebuilder:validation:Enum={"New","Pending","Upgrading","Upgraded", "Failed"}
	// This describe the status of the upgrade process
	Phase UpgradePhase `json:"phase"`

	Conditions []UpgradeCondition `json:"conditions,omitempty"`
	// +kubebuilder:validation:Optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// +kubebuilder:validation:Optional
	CompleteTime *metav1.Time `json:"completeTime,omitempty"`
}
type UpgradeConditionType string

const (
	UpgradeValidated UpgradeConditionType = "Validated"
	UpgradeCompleted  UpgradeConditionType = "Completed"
	UpgradeFailed    UpgradeConditionType = "Failed"
)

type UpgradeCondition struct {
	// Type of upgrade condition
	Type UpgradeConditionType `json:"type"`
	// Status of condition, one of True, False, Unknown
	Status v1.ConditionStatus `json:"status"`
	// Last time the condition was checked.
	// +kubebuilder:validation:Optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`
	// Last time the condition transit from one status to another.
	// +kubebuilder:validation:Optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// (brief) reason for the condition's last transition.
	// +kubebuilder:validation:Optional
	Reason string `json:"reason,omitempty"`
	// Human readable message indicating details about last transition.
	// +kubebuilder:validation:Optional
	Message string `json:"message,omitempty"`
}

type UpgradePhase string

const (
	UpgradePhaseNew       UpgradePhase = "New"
	UpgradePhasePending   UpgradePhase = "Pending"
	UpgradePhaseUpgrading UpgradePhase = "Upgrading"
	UpgradePhaseUpgraded  UpgradePhase = "Upgraded"
	UpgradePhaseFailed     UpgradePhase = "Failed"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// UpgradeConfig is the Schema for the upgradeconfigs API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=upgradeconfigs,scope=Cluster,shortName=upgrade
// +kubebuilder:printcolumn:name="status",type="string",JSONPath=".status.phase"
type UpgradeConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UpgradeConfigSpec   `json:"spec,omitempty"`
	Status UpgradeConfigStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// UpgradeConfigList contains a list of UpgradeConfig
type UpgradeConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UpgradeConfig `json:"items"`
}

// Update represents a release go gonna upgraded to
type Update struct {
	// Version of openshift release
	// +kubebuilder:validation:Type=string
	Version string `json:"version"`
	// Channel we gonna use for upgrades
	Channel string `json:"channel"`
	// +kubebuilder:default:=false
	// Force upgrade, default value is False
	Force bool `json:"force""`
}

// SubscriptionUpdate describe the 3rd party operator update config
type SubscriptionUpdate struct {
	// Describe the channel for the Subscription
	Channel string `json:"channel"`
	// Describe the namespace of the Subscription
	Namespace string `json:"namespace"`
	// Describe the name of the Subscription
	Name string `json:"name"`
}
// UpgradeTime defines a time point for an upgrade
type UpgradeTime struct {
	// +kubebuilder:validation:Enum={"Monday","Tuesday","Wednesday","Thursday", "Friday", "Saturday", "Sunday"}
	// Which Day of Week
	DayOfWeek WeekDay `json:"dayOfWeek"`

	// +kubebuilder:validation:Format:= "^([0-1][0-9]|[2][0-3]):([0-5][0-9])$"
	// Time in UTC like "01:00"
	TimeUTC string `json:"timeUtc"`
}

// UpgradeWindow describes the upgrade time window
type UpgradeWindow struct {
	MinimumUtc UpgradeTime   `json:"minimumUtc"`
	MaximumUtc UpgradeTime   `json:"maximumUtc"`
	Defaults   []UpgradeTime `json:"defaults"`
}

type WeekDay string

const (
	Monday    WeekDay = "Monday"
	Tuesday   WeekDay = "Tuesday"
	Wednesday WeekDay = "Wednesday"
	Thursday  WeekDay = "Thursday"
	Friday    WeekDay = "Friday"
	Saturday  WeekDay = "Saturday"
	Sunday    WeekDay = "Sunday"
)

// TimeUnit describes the time unit for defining a time duration
type TimeUnit string

const (
	TimeUnitDay   TimeUnit = "Day"
	TimeUnitWeek  TimeUnit = "Week"
	TimeUnitMonth TimeUnit = "Month"
)

// MaximumDuration describe the maximum duration of a time window
type MaximumDuration struct {
	// +kubebuilder:validation:Minimum=0
	Value int32 `json:"value"`
	// +kubebuilder:validation:Enum={"Day","Week","Month"}
	// Valid values are: "Day", "Week", "Month"
	UnitOfMeasure TimeUnit `json:"unitOfMeasure"`
}

// FreezeWindow describe the upgrade freeze time window
type FreezeWindow struct {
	// Maximum duration for a customer freeze window
	// +kubebuilder:validation:Optional
	MaximumDuration *MaximumDuration `json:"maximumDuration,omitempty"`

	// +kubebuilder:validation:Minimum=0

	// Maximum customerFreezeWindow can be created
	MaximumCount int32 `json:"maximumCount"`
}

func init() {
	SchemeBuilder.Register(&UpgradeConfig{}, &UpgradeConfigList{})
}

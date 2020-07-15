package v1alpha1

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpgradeConfigSpec defines the desired state of UpgradeConfig and upgrade window and freeze window
type UpgradeConfigSpec struct {
	// Specify the desired OpenShift release
	Desired Update `json:"desired"`

	// Specify the upgrade start time
	UpgradeAt string `json:"upgradeAt"`

	// +kubebuilder:default:=true
	// Given all conditions have passed and cluster is ready to upgrade, proceed governs this decision to continue and commence the upgrade
	Proceed bool `json:"proceed"`

	// This defines the 3rd party operator subscriptions upgrade
	// +kubebuilder:validation:Optional
	SubscriptionUpdates []SubscriptionUpdate `json:"subscriptionUpdates,omitempty"`
}

// UpgradeConfigStatus defines the observed state of UpgradeConfig
type UpgradeConfigStatus struct {

	// This record history of every upgrade
	// +kubebuilder:validation:Optional
	History UpgradeHistories `json:"history,omitempty"`
}

// Conditions is a set of Condition instances.
type UpgradeHistories []UpgradeHistory

// UpgradeHistory record history of upgrade
type UpgradeHistory struct {
	//Desired version of this upgrade
	Version string `json:"version,omitempty"`
	// +kubebuilder:validation:Enum={"New","Pending","Upgrading","Upgraded", "Failed"}
	// +kubebuilder:default:="New"
	// This describe the status of the upgrade process
	Phase UpgradePhase `json:"phase"`

	Conditions Conditions `json:"conditions,omitempty"`
	// +kubebuilder:validation:Optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// +kubebuilder:validation:Optional
	CompleteTime *metav1.Time `json:"completeTime,omitempty"`
}

type UpgradeConditionType string
type UpgradeCondition struct {
	// Type of upgrade condition
	Type UpgradeConditionType `json:"type"`
	// Status of condition, one of True, False, Unknown
	Status corev1.ConditionStatus `json:"status"`
	// Last time the condition was checked.
	// +kubebuilder:validation:Optional
	LastProbeTime *metav1.Time `json:"lastProbeTime,omitempty"`
	// Last time the condition transit from one status to another.
	// +kubebuilder:validation:Optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
	// Start time of this condition.
	// +kubebuilder:validation:Optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// Complete time of this condition.
	// +kubebuilder:validation:Optional
	CompleteTime *metav1.Time `json:"completeTime,omitempty"`
	// (brief) reason for the condition's last transition.
	// +kubebuilder:validation:Optional
	Reason string `json:"reason,omitempty"`
	// Human readable message indicating details about last transition.
	// +kubebuilder:validation:Optional
	Message string `json:"message,omitempty"`
}

const (
	UpgradeValidated              UpgradeConditionType = "Validation"
	UpgradePreHealthCheck         UpgradeConditionType = "PreHealthCheck"
	UpgradeScaleUpExtraNodes      UpgradeConditionType = "ScaleUpExtraNodes"
	ControlPlaneMaintWindow       UpgradeConditionType = "ControlPlaneMaintWindow"
	CommenceUpgrade               UpgradeConditionType = "CommenceUpgrade"
	ControlPlaneUpgraded          UpgradeConditionType = "ControlPlaneUpgraded"
	RemoveControlPlaneMaintWindow UpgradeConditionType = "RemoveControlPlaneMaintWindow"
	WorkersMaintWindow            UpgradeConditionType = "WorkersMaintWindow"
	AllWorkerNodesUpgraded        UpgradeConditionType = "AllWorkerNodesUpgraded"
	RemoveExtraScaledNodes        UpgradeConditionType = "RemoveExtraScaledNodes"
	UpdateSubscriptions           UpgradeConditionType = "UpdateSubscriptions"
	PostUpgradeVerification       UpgradeConditionType = "PostUpgradeVerification"
	RemoveMaintWindow             UpgradeConditionType = "RemoveMaintWindow"
	PostClusterHealthCheck        UpgradeConditionType = "PostClusterHealthCheck"
)

type UpgradePhase string

const (
	UpgradePhaseNew       UpgradePhase = "New"
	UpgradePhasePending   UpgradePhase = "Pending"
	UpgradePhaseUpgrading UpgradePhase = "Upgrading"
	UpgradePhaseUpgraded  UpgradePhase = "Upgraded"
	UpgradePhaseFailed    UpgradePhase = "Failed"
	UpgradePhaseUnknown   UpgradePhase = "Unknown"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// UpgradeConfig is the Schema for the upgradeconfigs API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=upgradeconfigs,scope=Cluster,shortName=upgrade
// +kubebuilder:printcolumn:name="desired_version",type="string",JSONPath=".spec.desired.version"
// +kubebuilder:printcolumn:name="phase",type="string",JSONPath=".status.history[0].phase"
// +kubebuilder:printcolumn:name="stage",type="string",JSONPath=".status.history[0].conditions[0].type"
// +kubebuilder:printcolumn:name="status",type="string",JSONPath=".status.history[0].conditions[0].status"
// +kubebuilder:printcolumn:name="reason",type="string",JSONPath=".status.history[0].conditions[0].reason"
// +kubebuilder:printcolumn:name="message",type="string",JSONPath=".status.history[0].conditions[0].message"
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
	Force bool `json:"force"`
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

// IsTrue Condition whether the condition status is "True".
func (c UpgradeCondition) IsTrue() bool {
	return c.Status == corev1.ConditionTrue
}

// IsFalse returns whether the condition status is "False".
func (c UpgradeCondition) IsFalse() bool {
	return c.Status == corev1.ConditionFalse
}

// IsUnknown returns whether the condition status is "Unknown".
func (c UpgradeCondition) IsUnknown() bool {
	return c.Status == corev1.ConditionUnknown
}

// DeepCopyInto copies in into out.
func (c *UpgradeCondition) DeepCopyInto(cpy *UpgradeCondition) {
	*cpy = *c
}

// Conditions is a set of Condition instances.
type Conditions []UpgradeCondition

// NewConditions initializes a set of conditions with the given list of
// conditions.
func NewConditions(conds ...UpgradeCondition) Conditions {
	conditions := Conditions{}
	for _, c := range conds {
		conditions.SetCondition(c)
	}
	return conditions
}

// IsTrueFor searches the set of conditions for a condition with the given
// ConditionType. If found, it returns `condition.IsTrue()`. If not found,
// it returns false.
func (conditions Conditions) IsTrueFor(t UpgradeConditionType) bool {
	for _, condition := range conditions {
		if condition.Type == t {
			return condition.IsTrue()
		}
	}
	return false
}

// IsFalseFor searches the set of conditions for a condition with the given
// ConditionType. If found, it returns `condition.IsFalse()`. If not found,
// it returns false.
func (conditions Conditions) IsFalseFor(t UpgradeConditionType) bool {
	for _, condition := range conditions {
		if condition.Type == t {
			return condition.IsFalse()
		}
	}
	return false
}

// IsUnknownFor searches the set of conditions for a condition with the given
// ConditionType. If found, it returns `condition.IsUnknown()`. If not found,
// it returns true.
func (conditions Conditions) IsUnknownFor(t UpgradeConditionType) bool {
	for _, condition := range conditions {
		if condition.Type == t {
			return condition.IsUnknown()
		}
	}
	return true
}

// SetCondition adds (or updates) the set of conditions with the given
// condition. It returns a boolean value indicating whether the set condition
// is new or was a change to the existing condition with the same type.
func (conditions *Conditions) SetCondition(newCond UpgradeCondition) bool {
	newCond.LastTransitionTime = &metav1.Time{Time: time.Now()}
	newCond.LastProbeTime = &metav1.Time{Time: time.Now()}
	for i, condition := range *conditions {
		if condition.Type == newCond.Type {
			if condition.Status == newCond.Status {
				newCond.LastTransitionTime = condition.LastTransitionTime
			}
			changed := condition.Status != newCond.Status ||
				condition.Reason != newCond.Reason ||
				condition.Message != newCond.Message

			(*conditions)[i] = newCond
			return changed
		}
	}
	*conditions = append([]UpgradeCondition{newCond}, *conditions...)
	return true
}

// GetCondition searches the set of conditions for the condition with the given
// ConditionType and returns it. If the matching condition is not found,
// GetCondition returns nil.
func (conditions Conditions) GetCondition(t UpgradeConditionType) *UpgradeCondition {
	for _, condition := range conditions {
		if condition.Type == t {
			return &condition
		}
	}
	return nil
}

// RemoveCondition removes the condition with the given ConditionType from
// the conditions set. If no condition with that type is found, RemoveCondition
// returns without performing any action. If the passed condition type is not
// found in the set of conditions, RemoveCondition returns false.
func (conditions *Conditions) RemoveCondition(t UpgradeConditionType) bool {
	if conditions == nil {
		return false
	}
	for i, condition := range *conditions {
		if condition.Type == t {
			*conditions = append((*conditions)[:i], (*conditions)[i+1:]...)
			return true
		}
	}
	return false
}

func (histories UpgradeHistories) GetHistory(version string) *UpgradeHistory {
	for _, history := range histories {
		if history.Version == version {
			return &history
		}
	}
	return nil
}
func (histories *UpgradeHistories) SetHistory(history UpgradeHistory) {
	for i, h := range *histories {
		if h.Version == history.Version {
			(*histories)[i] = history
			return
		}
	}
	*histories = append([]UpgradeHistory{history}, *histories...)
}
func init() {
	SchemeBuilder.Register(&UpgradeConfig{}, &UpgradeConfigList{})
}

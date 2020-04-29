package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PreferredUpgradeStartTimeSpec defines the desired state of PreferredUpgradeStartTime
type PreferredUpgradeStartTimeSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// +kubebuilder:validation:Enum={"Monday","Tuesday","Wednesday","Thursday", "Friday", "Saturday", "Sunday"}
	DayOfWeek WeekDay `json:"dayOfWeek"`

	// +kubebuilder:validation:Format:= "^([0-1][0-9]|[2][0-3]):([0-5][0-9])$"
	// Time in UTC like "01:00"
	TimeUtc string `json:"timeUtc"`
}

// PreferredUpgradeStartTimeStatus defines the observed state of PreferredUpgradeStartTime
type PreferredUpgradeStartTimeStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PreferredUpgradeStartTime is the Schema for the preferredupgradestarttimes API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=preferredupgradestarttimes,scope=Namespaced
type PreferredUpgradeStartTime struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PreferredUpgradeStartTimeSpec   `json:"spec,omitempty"`
	Status PreferredUpgradeStartTimeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PreferredUpgradeStartTimeList contains a list of PreferredUpgradeStartTime
type PreferredUpgradeStartTimeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PreferredUpgradeStartTime `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PreferredUpgradeStartTime{}, &PreferredUpgradeStartTimeList{})
}

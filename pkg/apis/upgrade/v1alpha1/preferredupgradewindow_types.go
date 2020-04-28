package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PreferredUpgradeWindowSpec defines the desired state of PreferredUpgradeWindow
type PreferredUpgradeWindowSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// +kubebuilder:validation:Enum={"Monday","Tuesday","Wednesday","Thursday", "Friday", "Saturday", "Sunday"}
	DayOfWeek WeekDay `json:"dayOfWeek"`

	// +kubebuilder:validation:Format:= "^([0-1][0-9]|[2][0-3]):([0-5][0-9])$"
	// Time in UTC like "01:00"
	TimeUtc string `json:"timeUtc"`
}

// PreferredUpgradeWindowStatus defines the observed state of PreferredUpgradeWindow
type PreferredUpgradeWindowStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PreferredUpgradeWindow is the Schema for the preferredupgradewindows API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=preferredupgradewindows,scope=Cluster
type PreferredUpgradeWindow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PreferredUpgradeWindowSpec   `json:"spec,omitempty"`
	Status PreferredUpgradeWindowStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PreferredUpgradeWindowList contains a list of PreferredUpgradeWindow
type PreferredUpgradeWindowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PreferredUpgradeWindow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PreferredUpgradeWindow{}, &PreferredUpgradeWindowList{})
}

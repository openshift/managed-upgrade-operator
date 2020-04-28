package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CustomerFreezeWindowSpec defines the desired state of CustomerFreezeWindow
type CustomerFreezeWindowSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	FromUtc UtcTime `json:"fromUtc"`

	ToUtc UtcTime `json:"toUtc"`
}

// CustomerFreezeWindowStatus defines the observed state of CustomerFreezeWindow
type CustomerFreezeWindowStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CustomerFreezeWindow is the Schema for the customerfreezewindows API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=customerfreezewindows,scope=Cluster
type CustomerFreezeWindow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CustomerFreezeWindowSpec   `json:"spec,omitempty"`
	Status CustomerFreezeWindowStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CustomerFreezeWindowList contains a list of CustomerFreezeWindow
type CustomerFreezeWindowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CustomerFreezeWindow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CustomerFreezeWindow{}, &CustomerFreezeWindowList{})
}

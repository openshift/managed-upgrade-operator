package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AdminFreezeWindowSpec defines a freeze window set by SRE
type AdminFreezeWindowSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	FromUtc UtcTime `json:"fromUtc"`

	ToUtc UtcTime `json:"toUtc"`
}

// AdminFreezeWindowStatus defines the observed state of AdminFreezeWindow
type AdminFreezeWindowStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AdminFreezeWindow is the Schema for the adminfreezewindows API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=adminfreezewindows,scope=Cluster
type AdminFreezeWindow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AdminFreezeWindowSpec   `json:"spec,omitempty"`
	Status AdminFreezeWindowStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AdminFreezeWindowList contains a list of AdminFreezeWindow
type AdminFreezeWindowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AdminFreezeWindow `json:"items"`
}

type Month string

const (
	January   Month = "January"
	February  Month = "February"
	March     Month = "March"
	April     Month = "April"
	May       Month = "May"
	June      Month = "June"
	July      Month = "July"
	August    Month = "August"
	September Month = "September"
	October   Month = "October"
	November  Month = "November"
	December  Month = "December"
)

type UtcTime struct {

	// +kubebuilder:validation:Enum={"January","February","March","April", "May", "June", "July","August", "September","October","November","December"}
	Month Month `json:"month"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4
	// +kubebuilder:validation:Optional
	WeekOfMonth *int `json:"weekOfMonth,omitempty"`

	// +kubebuilder:validation:Enum={"Monday","Tuesday","Wednesday","Thursday", "Friday", "Saturday", "Sunday"}
	// +kubebuilder:validation:Optional
	DayOfWeek *WeekDay `json:"dayOfWeek,omitempty"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=31
	// +kubebuilder:validation:Optional
	DayOfMonth *int `json:"dayOfMonth,omitempty"`

	// +kubebuilder:validation:Format:= "^([0-1][0-9]|[2][0-3]):([0-5][0-9])$"
	// Time in UTC like "01:00"
	Time string `json:"time"`

	// +kubebuilder:validation:Optional
	TimeStamp *metav1.Time `json:"timestamp,omitempty"`
}

func init() {
	SchemeBuilder.Register(&AdminFreezeWindow{}, &AdminFreezeWindowList{})
}

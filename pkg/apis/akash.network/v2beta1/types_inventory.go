package v2beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type InventoryState string

const (
	InventoryStatePulled = InventoryState("PULLED")
	InventoryStateError  = InventoryState("ERROR")
)

// InventoryRequest
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type InventoryRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   InventoryRequestSpec   `json:"spec,omitempty"`
	Status InventoryRequestStatus `json:"status,omitempty"`
}

type InventoryRequestSpec struct {
	Name string `json:"name"`
}

type InventoryRequestStatus struct {
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

// InventoryRequestList stores metadata and items list of storage class states
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type InventoryRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []InventoryRequest `json:"items"`
}

// Inventory
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Inventory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   InventorySpec   `json:"spec,omitempty"`
	Status InventoryStatus `json:"status,omitempty"`
}

// InventoryList stores metadata and items list of storage class states
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type InventoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Inventory `json:"items"`
}

type InventoryStatus struct {
	State    InventoryState `json:"state,omitempty"`
	Messages []string       `json:"message,omitempty"`
}

type InventoryClusterStorage struct {
	Class        string `json:"class,omitempty"`
	ResourcePair `json:",inline"`
}

type InventorySpec struct {
	Storage []InventoryClusterStorage `json:"storage"`
}

type ResourcePair struct {
	Allocatable uint64 `json:"allocatable"`
	Allocated   uint64 `json:"allocated"`
}

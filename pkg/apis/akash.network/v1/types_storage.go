package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type StorageClassInfo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   StorageClassInfoSpec   `json:"spec,omitempty"`
	Status StorageClassInfoStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StorageClassInfoList stores metadata and items list of storage class states
type StorageClassInfoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []StorageClassInfo `json:"items"`
}

type StorageClassInfoStatus struct {
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

type StorageClassInfoSpec struct {
	Capacity int64 `json:"capacity,omitempty"`
}

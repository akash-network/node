package v1

import (
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Manifest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ManifestSpec
	Status            ManifestStatus
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type LeaseID struct {
	metav1.TypeMeta `json:",inline"`
	// deployment address
	Deployment base.Bytes `protobuf:"bytes,1,opt,name=deployment,proto3,customtype=github.com/ovrclk/akash/types/base.Bytes" json:"deployment"`
	// deployment group sequence
	Group uint64 `protobuf:"varint,2,opt,name=group,proto3" json:"group,omitempty"`
	// order sequence
	Order uint64 `protobuf:"varint,3,opt,name=order,proto3" json:"order,omitempty"`
	// provider address
	Provider base.Bytes `protobuf:"bytes,4,opt,name=provider,proto3,customtype=github.com/ovrclk/akash/types/base.Bytes" json:"provider"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ManifestSpec struct {
	metav1.TypeMeta `json:",inline"`
	LID             LeaseID `json:"LeaseID"`
	// Placement profile name
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Service definitions
	Services []*ManifestService `protobuf:"bytes,2,rep,name=services" json:"services,omitempty"`
}

type ManifestService struct {
	// Service name
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Docker image
	Image string   `protobuf:"bytes,2,opt,name=image,proto3" json:"image,omitempty"`
	Args  []string `protobuf:"bytes,3,rep,name=args" json:"args,omitempty"`
	Env   []string `protobuf:"bytes,4,rep,name=env" json:"env,omitempty"`
	// Resource requirements
	Unit ResourceUnit `protobuf:"bytes,5,opt,name=unit" json:"unit"`
	// Number of instances
	Count uint32 `protobuf:"varint,6,opt,name=count,proto3" json:"count,omitempty"`
	// Overlay Network Links
	Expose []*ManifestServiceExpose `protobuf:"bytes,7,rep,name=expose" json:"expose,omitempty"`
}

type ManifestServiceExpose struct {
	Port         uint32 `protobuf:"varint,1,opt,name=port,proto3" json:"port,omitempty"`
	ExternalPort uint32 `protobuf:"varint,2,opt,name=externalPort,proto3" json:"externalPort,omitempty"`
	Proto        string `protobuf:"bytes,3,opt,name=proto,proto3" json:"proto,omitempty"`
	Service      string `protobuf:"bytes,4,opt,name=service,proto3" json:"service,omitempty"`
	Global       bool   `protobuf:"varint,5,opt,name=global,proto3" json:"global,omitempty"`
	// accepted hostnames
	Hosts []string `protobuf:"bytes,6,rep,name=hosts" json:"hosts,omitempty"`
}

// BEGIN EXCHANGE
type ResourceUnit struct {
	Cpu    uint32 `protobuf:"varint,1,opt,name=cpu,proto3" json:"cpu,omitempty"`
	Memory uint32 `protobuf:"varint,2,opt,name=memory,proto3" json:"memory,omitempty"`
	Disk   uint64 `protobuf:"varint,3,opt,name=disk,proto3" json:"disk,omitempty"`
}

type ManifestStatus struct {
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ManifestList struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Items             []Manifest `json:"items"`
}

func (m *Manifest) LeaseID() types.LeaseID {
	return types.LeaseID{
		Deployment: m.Spec.LID.Deployment,
		Group:      m.Spec.LID.Group,
		Order:      m.Spec.LID.Order,
		Provider:   m.Spec.LID.Provider,
	}
}

func (m *Manifest) ManifestGroup() *types.ManifestGroup {
	json, err := m.Spec.Marshal()
	if err != nil {
		panic(err.Error())
	}
	group := &types.ManifestGroup{}
	err = group.Unmarshal(json)
	if err != nil {
		panic(err.Error())
	}
	return group
}

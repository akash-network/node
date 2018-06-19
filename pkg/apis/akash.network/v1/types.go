package v1

import (
	"bytes"
	"encoding/json"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/ovrclk/akash/types"
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

type ManifestGroup struct {
	metav1.TypeMeta `json:",inline"`
	// Placement profile name
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Service definitions
	Services []*ManifestService `protobuf:"bytes,2,rep,name=services" json:"services,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LeaseID struct {
	metav1.TypeMeta `json:",inline"`
	// deployment address
	Deployment []byte `protobuf:"bytes,1,opt,name=deployment,proto3,customtype=github.com/ovrclk/akash/types/base.Bytes" json:"deployment"`
	// deployment group sequence
	Group uint64 `protobuf:"varint,2,opt,name=group,proto3" json:"group,omitempty"`
	// order sequence
	Order uint64 `protobuf:"varint,3,opt,name=order,proto3" json:"order,omitempty"`
	// provider address
	Provider []byte `protobuf:"bytes,4,opt,name=provider,proto3,customtype=github.com/ovrclk/akash/types/base.Bytes" json:"provider"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ManifestSpec struct {
	metav1.TypeMeta `json:",inline"`
	LeaseID         LeaseID       `json:"lease_id"`
	ManifestGroup   ManifestGroup `json:"manifest_group"`
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

func (m Manifest) ManifestGroup() *types.ManifestGroup {
	group, err := m.manifestGroup()
	if err != nil {
		panic("kube manifest manifestGroup error: " + err.Error())
	}
	return group
}

func (m *Manifest) manifestGroup() (*types.ManifestGroup, error) {
	group := &types.ManifestGroup{}
	unmarshaler := &jsonpb.Unmarshaler{}
	buf, err := json.Marshal(m.Spec.ManifestGroup)
	if err != nil {
		return nil, err
	}
	err = unmarshaler.Unmarshal(bytes.NewReader(buf), group)
	if err != nil {
		return nil, err
	}
	return group, nil
}

func (m Manifest) LeaseID() types.LeaseID {
	leaseID, err := m.leaseID()
	if err != nil {
		panic("kube manifest leaseID error: " + err.Error())
	}
	return leaseID
}

func (m *Manifest) leaseID() (types.LeaseID, error) {
	leaseID := types.LeaseID{}
	buf, err := json.Marshal(m.Spec.LeaseID)
	if err != nil {
		return leaseID, err
	}
	unmarshaler := &jsonpb.Unmarshaler{}
	err = unmarshaler.Unmarshal(bytes.NewReader(buf), &leaseID)
	if err != nil {
		return leaseID, err
	}
	return leaseID, nil
}

func NewManifest(name string, lid *types.LeaseID, mgroup *types.ManifestGroup) (*Manifest, error) {
	buf := bytes.NewBuffer(nil)
	marshaler := &jsonpb.Marshaler{}
	manifestGroup := &ManifestGroup{}
	leaseID := &LeaseID{}

	err := marshaler.Marshal(buf, mgroup)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(buf.Bytes(), manifestGroup)
	if err != nil {
		return nil, err
	}

	buf.Reset()
	err = marshaler.Marshal(buf, lid)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(buf.Bytes(), leaseID)
	if err != nil {
		return nil, err
	}

	return &Manifest{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Manifest",
			APIVersion: "akash.network/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: ManifestSpec{
			ManifestGroup: *manifestGroup,
			LeaseID:       *leaseID,
		},
	}, nil
}

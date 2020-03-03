package v1

import (
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Manifest store metadata, specifications and status of manifest
type Manifest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ManifestSpec
	Status            ManifestStatus
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ManifestGroup store metadata, name and list of manifest services
type ManifestGroup struct {
	metav1.TypeMeta `json:",inline"`
	// Placement profile name
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Service definitions
	Services []*ManifestService `protobuf:"bytes,2,rep,name=services" json:"services,omitempty"`
}

// ToAkash returns akash group details formatted from manifest group
func (m ManifestGroup) ToAkash() *manifest.Group {
	ma := &manifest.Group{Name: m.Name}

	for _, svc := range m.Services {
		masvc := manifest.Service{
			Name:  svc.Name,
			Image: svc.Image,
			Args:  svc.Args[:],
			Env:   svc.Env[:],
			Unit: types.Unit{
				CPU:     svc.Unit.CPU,
				Memory:  svc.Unit.Memory,
				Storage: svc.Unit.Storage,
			},
			Count: svc.Count,
		}
		for _, expose := range svc.Expose {
			masvc.Expose = append(masvc.Expose, manifest.ServiceExpose{
				Port:         expose.Port,
				ExternalPort: expose.ExternalPort,
				Proto:        expose.Proto,
				Service:      expose.Service,
				Global:       expose.Global,
				Hosts:        expose.Hosts[:],
			})
		}

		ma.Services = append(ma.Services, masvc)
	}

	return ma
}

// ManifestGroupFromAkash returns manifest group instance from akash group
func ManifestGroupFromAkash(m *manifest.Group) ManifestGroup {
	ma := ManifestGroup{Name: m.Name}

	for _, svc := range m.Services {
		masvc := &ManifestService{
			Name:  svc.Name,
			Image: svc.Image,
			Args:  svc.Args[:],
			Env:   svc.Env[:],
			Unit: ResourceUnit{
				CPU:     svc.Unit.CPU,
				Memory:  svc.Unit.Memory,
				Storage: svc.Unit.Storage,
			},
			Count: svc.Count,
		}
		for _, expose := range svc.Expose {
			masvc.Expose = append(masvc.Expose, &ManifestServiceExpose{
				Port:         expose.Port,
				ExternalPort: expose.ExternalPort,
				Proto:        expose.Proto,
				Service:      expose.Service,
				Global:       expose.Global,
				Hosts:        expose.Hosts[:],
			})
		}

		ma.Services = append(ma.Services, masvc)
	}

	return ma
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LeaseID stores deployment, group sequence, order, provider and metadata
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

// ToAkash returns LeaseID from LeaseID details
func (id LeaseID) ToAkash() mtypes.LeaseID {
	return mtypes.LeaseID{
		// TODO
		// Deployment: id.Deployment,
		// Group:      id.Group,
		// Order:      id.Order,
		// Provider:   id.Provider,
	}
}

// LeaseIDFromAkash returns LeaseID instance from akash
func LeaseIDFromAkash(id mtypes.LeaseID) LeaseID {
	return LeaseID{
		// TODO
		// Deployment: id.Deployment,
		// Group:      id.Group,
		// Order:      id.Order,
		// Provider:   id.Provider,
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ManifestSpec stores LeaseID, Group and metadata details
type ManifestSpec struct {
	metav1.TypeMeta `json:",inline"`
	LeaseID         LeaseID       `json:"lease_id"`
	ManifestGroup   ManifestGroup `json:"manifest_group"`
}

// ManifestService stores name, image, args, env, unit, count and expose list of service
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

// ManifestServiceExpose stores exposed ports and accepted hosts details
type ManifestServiceExpose struct {
	Port         uint32 `protobuf:"varint,1,opt,name=port,proto3" json:"port,omitempty"`
	ExternalPort uint32 `protobuf:"varint,2,opt,name=externalPort,proto3" json:"externalPort,omitempty"`
	Proto        string `protobuf:"bytes,3,opt,name=proto,proto3" json:"proto,omitempty"`
	Service      string `protobuf:"bytes,4,opt,name=service,proto3" json:"service,omitempty"`
	Global       bool   `protobuf:"varint,5,opt,name=global,proto3" json:"global,omitempty"`
	// accepted hostnames
	Hosts []string `protobuf:"bytes,6,rep,name=hosts" json:"hosts,omitempty"`
}

// ResourceUnit stores cpu, memory and storage details
type ResourceUnit struct {
	CPU     uint32 `protobuf:"varint,1,opt,name=CPU,proto3" json:"CPU,omitempty"`
	Memory  uint64 `protobuf:"varint,2,opt,name=memory,proto3" json:"memory,omitempty"`
	Storage uint64 `protobuf:"varint,3,opt,name=storage,proto3" json:"storage,omitempty"`
}

// ManifestStatus stores state and message of manifest
type ManifestStatus struct {
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ManifestList stores metadata and items list of manifest
type ManifestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:",inline"`
	Items           []Manifest `json:"items"`
}

// ManifestGroup returns Group details from manifest
func (m Manifest) ManifestGroup() manifest.Group {
	return *m.Spec.ManifestGroup.ToAkash()
}

// LeaseID returns lease details from manifest
func (m Manifest) LeaseID() mtypes.LeaseID {
	return m.Spec.LeaseID.ToAkash()
}

// NewManifest creates new manifest with provided details. Returns error incase of failure.
func NewManifest(name string, lid mtypes.LeaseID, mgroup *manifest.Group) (*Manifest, error) {
	return &Manifest{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Manifest",
			APIVersion: "akash.network/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: ManifestSpec{
			ManifestGroup: ManifestGroupFromAkash(mgroup),
			LeaseID:       LeaseIDFromAkash(lid),
		},
	}, nil
}

package v1

import (
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Manifest store metadata, specifications and status of the Lease
type Manifest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   ManifestSpec   `json:"spec,omitempty"`
	Status ManifestStatus `json:"status,omitempty"`
}

// ManifestStatus stores state and message of manifest
type ManifestStatus struct {
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

// ManifestSpec stores LeaseID, Group and metadata details
type ManifestSpec struct {
	LeaseID LeaseID       `json:"lease-id"`
	Group   ManifestGroup `json:"group"`
}

// Deployment returns the cluster.Deployment that the saved manifest represents.
func (m Manifest) Deployment() (cluster.Deployment, error) {
	lid, err := m.Spec.LeaseID.toAkash()
	if err != nil {
		return nil, err
	}

	group, err := m.Spec.Group.toAkash()
	if err != nil {
		return nil, err
	}
	return deployment{lid: lid, group: group}, nil
}

type deployment struct {
	lid   mtypes.LeaseID
	group manifest.Group
}

func (d deployment) LeaseID() mtypes.LeaseID {
	return d.lid
}

func (d deployment) ManifestGroup() manifest.Group {
	return d.group
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
			Group:   manifestGroupFromAkash(mgroup),
			LeaseID: leaseIDFromAkash(lid),
		},
	}, nil
}

// LeaseID stores deployment, group sequence, order, provider and metadata
type LeaseID struct {
	Owner    string `json:"owner"`
	DSeq     string `json:"dseq"`
	GSeq     uint32 `json:"gseq"`
	OSeq     uint32 `json:"oseq"`
	Provider string `json:"provider"`
}

// ToAkash returns LeaseID from LeaseID details
func (id LeaseID) toAkash() (mtypes.LeaseID, error) {
	owner, err := sdk.AccAddressFromBech32(id.Owner)
	if err != nil {
		return mtypes.LeaseID{}, err
	}

	provider, err := sdk.AccAddressFromBech32(id.Provider)
	if err != nil {
		return mtypes.LeaseID{}, err
	}

	dseq, err := strconv.ParseUint(id.DSeq, 10, 64)
	if err != nil {
		return mtypes.LeaseID{}, err
	}

	return mtypes.LeaseID{
		Owner:    owner,
		DSeq:     dseq,
		GSeq:     id.GSeq,
		OSeq:     id.OSeq,
		Provider: provider,
	}, nil
}

// LeaseIDFromAkash returns LeaseID instance from akash
func leaseIDFromAkash(id mtypes.LeaseID) LeaseID {
	return LeaseID{
		Owner:    id.Owner.String(),
		DSeq:     strconv.FormatUint(id.DSeq, 10),
		GSeq:     id.GSeq,
		OSeq:     id.OSeq,
		Provider: id.Provider.String(),
	}
}

// ManifestGroup stores metadata, name and list of SDL manifest services
type ManifestGroup struct {
	// Placement profile name
	Name string `json:"name,omitempty"`
	// Service definitions
	Services []ManifestService `json:"services,omitempty"`
}

// ToAkash returns akash group details formatted from manifest group
func (m ManifestGroup) toAkash() (manifest.Group, error) {
	am := manifest.Group{
		Name:     m.Name,
		Services: make([]manifest.Service, 0, len(m.Services)),
	}

	for _, svc := range m.Services {
		asvc, err := svc.toAkash()
		if err != nil {
			return am, err
		}
		am.Services = append(am.Services, asvc)
	}

	return am, nil
}

// ManifestGroupFromAkash returns manifest group instance from akash group
func manifestGroupFromAkash(m *manifest.Group) ManifestGroup {
	ma := ManifestGroup{
		Name:     m.Name,
		Services: make([]ManifestService, 0, len(m.Services)),
	}

	for _, svc := range m.Services {
		ma.Services = append(ma.Services, manifestServiceFromAkash(svc))
	}

	return ma
}

// ManifestService stores name, image, args, env, unit, count and expose list of service
type ManifestService struct {
	// Service name
	Name string `json:"name,omitempty"`
	// Docker image
	Image string   `json:"image,omitempty"`
	Args  []string `json:"args,omitempty"`
	Env   []string `json:"env,omitempty"`
	// Resource requirements
	Unit ResourceUnit `json:"unit"`
	// Number of instances
	Count uint32 `json:"count,omitempty"`
	// Overlay Network Links
	Expose []ManifestServiceExpose `json:"expose,omitempty"`
}

func (ms ManifestService) toAkash() (manifest.Service, error) {
	unit, err := ms.Unit.toAkash()
	if err != nil {
		return manifest.Service{}, err
	}
	ams := &manifest.Service{
		Name:   ms.Name,
		Image:  ms.Image,
		Args:   ms.Args,
		Env:    ms.Env,
		Unit:   unit,
		Count:  ms.Count,
		Expose: make([]manifest.ServiceExpose, 0, len(ms.Expose)),
	}

	for _, expose := range ms.Expose {
		ams.Expose = append(ams.Expose, expose.toAkash())
	}

	return *ams, nil
}

func manifestServiceFromAkash(ams manifest.Service) ManifestService {
	ms := ManifestService{
		Name:   ams.Name,
		Image:  ams.Image,
		Args:   ams.Args,
		Env:    ams.Env,
		Unit:   resourceUnitFromAkash(ams.Unit),
		Count:  ams.Count,
		Expose: make([]ManifestServiceExpose, 0, len(ams.Expose)),
	}

	for _, expose := range ams.Expose {
		ms.Expose = append(ms.Expose, manifestServiceExposeFromAkash(expose))
	}

	return ms
}

// ManifestServiceExpose stores exposed ports and accepted hosts details
type ManifestServiceExpose struct {
	Port         uint16 `json:"port,omitempty"`
	ExternalPort uint16 `json:"external-port,omitempty"`
	Proto        string `json:"proto,omitempty"`
	Service      string `json:"service,omitempty"`
	Global       bool   `json:"global,omitempty"`
	// accepted hostnames
	Hosts []string `json:"hosts,omitempty"`
}

func (mse ManifestServiceExpose) toAkash() manifest.ServiceExpose {
	return manifest.ServiceExpose{
		Port:         mse.Port,
		ExternalPort: mse.ExternalPort,
		Proto:        mse.Proto,
		Service:      mse.Service,
		Global:       mse.Global,
		Hosts:        mse.Hosts,
	}
}

func manifestServiceExposeFromAkash(amse manifest.ServiceExpose) ManifestServiceExpose {
	return ManifestServiceExpose{
		Port:         amse.Port,
		ExternalPort: amse.ExternalPort,
		Proto:        amse.Proto,
		Service:      amse.Service,
		Global:       amse.Global,
		Hosts:        amse.Hosts,
	}
}

// ResourceUnit stores cpu, memory and storage details
type ResourceUnit struct {
	CPU     uint32 `json:"cpu,omitempty"`
	Memory  string `json:"memory,omitempty"`
	Storage string `json:"storage,omitempty"`
}

func (ru ResourceUnit) toAkash() (types.Unit, error) {
	memory, err := strconv.ParseUint(ru.Memory, 10, 64)
	if err != nil {
		return types.Unit{}, err
	}
	storage, err := strconv.ParseUint(ru.Storage, 10, 64)
	if err != nil {
		return types.Unit{}, err
	}

	return types.Unit{
		CPU:     ru.CPU,
		Memory:  memory,
		Storage: storage,
	}, nil
}

func resourceUnitFromAkash(aru types.Unit) ResourceUnit {
	return ResourceUnit{
		CPU:     aru.CPU,
		Memory:  strconv.FormatUint(aru.Memory, 10),
		Storage: strconv.FormatUint(aru.Storage, 10),
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ManifestList stores metadata and items list of manifest
type ManifestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:",inline"`
	Items           []Manifest `json:"items"`
}

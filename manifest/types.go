package manifest

import (
	"github.com/ovrclk/akash/types"
)

// Manifest store list of groups
type Manifest []Group

type ServiceProtocol string

const (
	TCP = ServiceProtocol("TCP")
	UDP = ServiceProtocol("UDP")
)

func (sp ServiceProtocol) ToString() string {
	return string(sp)
}

// GetGroups returns a manifest with groups list
func (m Manifest) GetGroups() []Group {
	return m
}

// Group store name and list of services
type Group struct {
	Name     string
	Services []Service
}

// GetName returns the name of group
func (g Group) GetName() string {
	return g.Name
}

// GetResources returns list of resources in a group
func (g Group) GetResources() []types.Resources {
	resources := make([]types.Resources, 0, len(g.Services))
	for _, s := range g.Services {
		resources = append(resources, types.Resources{
			Resources: s.Resources,
			Count:     s.Count,
		})
	}
	return resources
}

type StorageParams struct {
	Name     string `json:"name" yaml:"name"`
	Mount    string `json:"readOnly" yaml:"mount"`
	ReadOnly bool   `json:"mount" yaml:"readOnly"`
}

type ServiceParams struct {
	Storage []StorageParams
}

// Service stores name, image, args, env, unit, count and expose list of service
type Service struct {
	Name      string
	Image     string
	Command   []string
	Args      []string
	Env       []string
	Resources types.ResourceUnits
	Count     uint32
	Expose    []ServiceExpose
	Params    *ServiceParams `json:"params,omitempty" yaml:"params,omitempty"`
}

// GetResourceUnits returns resources unit of service
func (s Service) GetResourceUnits() types.ResourceUnits {
	return s.Resources
}

// GetCount returns count of service
func (s Service) GetCount() uint32 {
	return s.Count
}

// ServiceExpose stores exposed ports and hosts details
type ServiceExpose struct {
	Port         uint16 // Port on the container
	ExternalPort uint16 // Port on the service definition
	Proto        ServiceProtocol
	Service      string
	Global       bool
	Hosts        []string
	HTTPOptions  ServiceExposeHTTPOptions
}

type ServiceExposeHTTPOptions struct {
	MaxBodySize uint32
	ReadTimeout uint32
	SendTimeout uint32
	NextTries   uint32
	NextTimeout uint32
	NextCases   []string
}

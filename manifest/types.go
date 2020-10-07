package manifest

import (
	"github.com/ovrclk/akash/types"
)

// Manifest store list of groups
type Manifest []Group

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
}

// GetResourcesUnit returns resources unit of service
func (s Service) GetResourceUnits() types.ResourceUnits {
	return s.Resources
}

// GetCount returns count of service
func (s Service) GetCount() uint32 {
	return s.Count
}

// ServiceExpose stores exposed ports and hosts details
type ServiceExpose struct {
	Port         uint16
	ExternalPort uint16
	Proto        string
	Service      string
	Global       bool
	Hosts        []string
}

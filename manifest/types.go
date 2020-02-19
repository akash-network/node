package manifest

import (
	"github.com/ovrclk/akash/types"
)

type Manifest []Group

func (m Manifest) GetGroups() []Group {
	return m
}

type Group struct {
	Name     string
	Services []Service
}

func (g Group) GetName() string {
	return g.Name
}

func (g Group) GetResources() []types.Resource {
	resources := make([]types.Resource, 0, len(g.Services))
	for _, s := range g.Services {
		resources = append(resources, types.Resource{
			Unit:  s.Unit,
			Count: s.Count,
		})
	}
	return resources
}

type Service struct {
	Name   string
	Image  string
	Args   []string
	Env    []string
	Unit   types.Unit
	Count  uint32
	Expose []ServiceExpose
}

func (s Service) GetUnit() types.Unit {
	return s.Unit
}

func (s Service) GetCount() uint32 {
	return s.Count
}

type ServiceExpose struct {
	Port         uint32
	ExternalPort uint32
	Proto        string
	Service      string
	Global       bool
	Hosts        []string
}

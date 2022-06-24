package v2beta1

import (
	"errors"
	"fmt"
	types "github.com/ovrclk/akash/types/v1beta2"
	corev1 "k8s.io/api/core/v1"
)

// Manifest store list of groups
type Manifest []Group

type ServiceProtocol string

const (
	TCP = ServiceProtocol("TCP")
	UDP = ServiceProtocol("UDP")
)

var (
	errUnknownServiceProtocol = errors.New("unknown service protocol")
)

func (sp ServiceProtocol) ToString() string {
	return string(sp)
}

func (sp ServiceProtocol) ToKube() (corev1.Protocol, error) {
	switch sp {
	case TCP:
		return corev1.ProtocolTCP, nil
	case UDP:
		return corev1.ProtocolUDP, nil
	}

	return corev1.Protocol(""), fmt.Errorf("%w: %v", errUnknownServiceProtocol, sp)
}

func ServiceProtocolFromKube(proto corev1.Protocol) (ServiceProtocol, error) {
	switch proto {
	case corev1.ProtocolTCP:
		return TCP, nil
	case corev1.ProtocolUDP:
		return UDP, nil
	}

	return ServiceProtocol(""), fmt.Errorf("%w: %v", errUnknownServiceProtocol, proto)
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
	Port                   uint16 // Port on the container
	ExternalPort           uint16 // Port on the service definition
	Proto                  ServiceProtocol
	Service                string
	Global                 bool
	Hosts                  []string
	HTTPOptions            ServiceExposeHTTPOptions
	IP                     string // The name of the IP address associated with this, if any
	EndpointSequenceNumber uint32 // The sequence number of the associated endpoint in the on-chain data
}

type ServiceExposeHTTPOptions struct {
	MaxBodySize uint32
	ReadTimeout uint32
	SendTimeout uint32
	NextTries   uint32
	NextTimeout uint32
	NextCases   []string
}

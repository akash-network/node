package cluster

import mtypes "github.com/ovrclk/akash/x/market/types"

type LeaseIdHostnameConnection interface {
	GetLeaseID() mtypes.LeaseID
	GetHostname() string
	GetExternalPort() int32
	GetServiceName() string
}

type ActiveHostname struct {
	ID       mtypes.LeaseID
	Hostname string
}

type ProviderResourceEvent string

const (
	ProviderResourceAdd    = ProviderResourceEvent("add")
	ProviderResourceUpdate = ProviderResourceEvent("update")
	ProviderResourceDelete = ProviderResourceEvent("delete")
)

type HostnameResourceEvent interface {
	GetLeaseID() mtypes.LeaseID
	GetEventType() ProviderResourceEvent
	GetHostname() string
	GetServiceName() string
	GetExternalPort() uint32
}

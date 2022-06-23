package v1beta2

import (
	manifest "github.com/ovrclk/akash/manifest/v2beta1"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

type IPResourceEvent interface {
	GetLeaseID() mtypes.LeaseID
	GetServiceName() string
	GetExternalPort() uint32
	GetPort() uint32
	GetSharingKey() string
	GetProtocol() manifest.ServiceProtocol
	GetEventType() ProviderResourceEvent
}

type IPPassthrough interface {
	GetLeaseID() mtypes.LeaseID
	GetServiceName() string
	GetExternalPort() uint32
	GetPort() uint32
	GetSharingKey() string
	GetProtocol() manifest.ServiceProtocol
}

type IPLeaseState interface {
	IPPassthrough
	GetIP() string
}

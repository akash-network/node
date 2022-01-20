package v1beta2

import (
	manifest "github.com/ovrclk/akash/manifest/v2beta1"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

type ConnectHostnameToDeploymentDirective struct {
	Hostname    string
	LeaseID     mtypes.LeaseID
	ServiceName string
	ServicePort int32
	ReadTimeout uint32
	SendTimeout uint32
	NextTimeout uint32
	MaxBodySize uint32
	NextTries   uint32
	NextCases   []string
}

type ClusterIPPassthroughDirective struct {
	LeaseID      mtypes.LeaseID
	ServiceName  string
	Port         uint32
	ExternalPort uint32
	SharingKey   string
	Protocol     manifest.ServiceProtocol
}

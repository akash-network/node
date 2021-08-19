package cluster

import mtypes "github.com/ovrclk/akash/x/market/types"

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
	NextCases   []string //
}

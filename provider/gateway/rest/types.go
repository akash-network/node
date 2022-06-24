package rest

import (
	cltypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
)

type LeasedIPStatus struct {
	Port         uint32
	ExternalPort uint32
	Protocol     string
	IP           string
}

type LeaseStatus struct {
	Services       map[string]*cltypes.ServiceStatus        `json:"services"`
	ForwardedPorts map[string][]cltypes.ForwardedPortStatus `json:"forwarded_ports"` // Container services that are externally accessible
	IPs            map[string][]LeasedIPStatus              `json:"ips"`
}

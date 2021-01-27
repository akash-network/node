package provider

import (
	"github.com/ovrclk/akash/provider/bidengine"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	"github.com/ovrclk/akash/provider/manifest"
)

// Status is the data structure that stores Cluster, Bidengine and Manifest details.
type Status struct {
	Cluster               *ctypes.Status    `json:"cluster"`
	Bidengine             *bidengine.Status `json:"bidengine"`
	Manifest              *manifest.Status  `json:"manifest"`
	ClusterPublicHostname string            `json:"cluster_public_hostname,omitempty"`
}

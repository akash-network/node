package provider

import (
	"github.com/ovrclk/akash/provider/bidengine"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/manifest"
)

// Status is the data structure that stores Cluster, Bidengine and Manifest details.
type Status struct {
	Cluster   *cluster.Status
	Bidengine *bidengine.Status
	Manifest  *manifest.Status
}

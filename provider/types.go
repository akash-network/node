package provider

import (
	"github.com/ovrclk/akash/provider/bidengine"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/manifest"
)

type Status struct {
	Cluster   *cluster.Status
	Bidengine *bidengine.Status
	Manifest  *manifest.Status
}

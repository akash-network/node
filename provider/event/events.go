package event

import (
	"github.com/ovrclk/akash/types"
)

type LeaseWon struct {
	LeaseID types.LeaseID
	Group   *types.DeploymentGroup
	Price   uint32
}

type ManifestReceived struct {
	LeaseID    types.LeaseID
	Manifest   *types.Manifest
	Deployment *types.Deployment
}

package event

import (
	"github.com/ovrclk/akash/types"
)

type LeaseWon struct {
	LeaseID types.LeaseID
	Group   *types.DeploymentGroup
	Price   uint64
}

type ManifestReceived struct {
	LeaseID    types.LeaseID
	Manifest   *types.Manifest
	Deployment *types.Deployment
	Group      *types.DeploymentGroup
}

func (ev ManifestReceived) ManifestGroup() *types.ManifestGroup {
	for _, mgroup := range ev.Manifest.Groups {
		if mgroup.Name == ev.Group.Name {
			return mgroup
		}
	}
	return nil
}

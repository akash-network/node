package event

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/manifest"
	dquery "github.com/ovrclk/akash/x/deployment/query"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

type LeaseWon struct {
	LeaseID mtypes.LeaseID
	Group   *dquery.Group
	Price   sdk.Coin
}

type ManifestReceived struct {
	LeaseID    mtypes.LeaseID
	Manifest   *manifest.Manifest
	Deployment *dquery.Deployment
	Group      *dquery.Group
}

func (ev ManifestReceived) ManifestGroup() *manifest.Group {
	for _, mgroup := range *ev.Manifest {
		if mgroup.Name == ev.Group.Name {
			return &mgroup
		}
	}
	return nil
}

type ClusterDeploymentStatus string

const (
	ClusterDeploymentPending  ClusterDeploymentStatus = "pending"
	ClusterDeploymentDeployed ClusterDeploymentStatus = "deployed"
)

type ClusterDeployment struct {
	LeaseID mtypes.LeaseID
	Group   *manifest.Group
	Status  ClusterDeploymentStatus
}

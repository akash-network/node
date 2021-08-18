package event

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/manifest"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

// LeaseWon is the data structure that includes leaseID, group and price
type LeaseWon struct {
	LeaseID mtypes.LeaseID
	Group   *dtypes.Group
	Price   sdk.Coin
}

// ManifestReceived stores leaseID, manifest received, deployment and group details
// to be provisioned by the Provider.
type ManifestReceived struct {
	LeaseID    mtypes.LeaseID
	Manifest   *manifest.Manifest
	Deployment *dtypes.QueryDeploymentResponse
	Group      *dtypes.Group
}

// ManifestGroup returns group if present in manifest or nil
func (ev ManifestReceived) ManifestGroup() *manifest.Group {
	for _, mgroup := range *ev.Manifest {
		if mgroup.Name == ev.Group.GroupSpec.Name {
			mgroup := mgroup
			return &mgroup
		}
	}
	return nil
}

// ClusterDeploymentStatus represents status of the cluster deployment
type ClusterDeploymentStatus string

const (
	// ClusterDeploymentUpdated is used whenever the deployment in the cluster is updated but may not be functional
	ClusterDeploymentUpdated  ClusterDeploymentStatus = "updated"
	// ClusterDeploymentPending is used when cluster deployment status is pending
	ClusterDeploymentPending ClusterDeploymentStatus = "pending"
	// ClusterDeploymentDeployed is used when cluster deployment status is deployed
	ClusterDeploymentDeployed ClusterDeploymentStatus = "deployed"
)

// ClusterDeployment stores leaseID, group details and deployment status
type ClusterDeployment struct {
	LeaseID mtypes.LeaseID
	Group   *manifest.Group
	Status  ClusterDeploymentStatus
}

// Empty type used as a marker to indicate _any_ lease should be withdrawn now
type LeaseWithdrawNow struct{}

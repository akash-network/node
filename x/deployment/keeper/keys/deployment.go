package keys

import (
	"cosmossdk.io/collections"

	v1 "pkg.akt.dev/go/node/deployment/v1"
)

// DeploymentIDToKey converts a v1.DeploymentID to a DeploymentPrimaryKey for use with the IndexedMap
func DeploymentIDToKey(id v1.DeploymentID) DeploymentPrimaryKey {
	return collections.Join(id.Owner, id.DSeq)
}

// KeyToDeploymentID converts a DeploymentPrimaryKey back to a v1.DeploymentID
func KeyToDeploymentID(key DeploymentPrimaryKey) v1.DeploymentID {
	return v1.DeploymentID{
		Owner: key.K1(),
		DSeq:  key.K2(),
	}
}

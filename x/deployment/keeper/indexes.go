package keeper

import (
	"cosmossdk.io/collections"
	"cosmossdk.io/collections/indexes"

	v1 "pkg.akt.dev/go/node/deployment/v1"
	types "pkg.akt.dev/go/node/deployment/v1beta4"

	"pkg.akt.dev/node/x/deployment/keeper/keys"
)

// DeploymentIndexes defines the secondary indexes for the deployment IndexedMap
type DeploymentIndexes struct {
	// State indexes deployments by their state (Active, Closed)
	State *indexes.Multi[int32, keys.DeploymentPrimaryKey, v1.Deployment]
}

// GroupIndexes defines the secondary indexes for the group IndexedMap
type GroupIndexes struct {
	// State indexes groups by their state (Open, Paused, InsufficientFunds, Closed)
	State *indexes.Multi[int32, keys.GroupPrimaryKey, types.Group]

	// Deployment indexes groups by their parent deployment (owner, dseq) for GetGroups queries
	Deployment *indexes.Multi[keys.DeploymentPrimaryKey, keys.GroupPrimaryKey, types.Group]
}

func (d DeploymentIndexes) IndexesList() []collections.Index[keys.DeploymentPrimaryKey, v1.Deployment] {
	return []collections.Index[keys.DeploymentPrimaryKey, v1.Deployment]{
		d.State,
	}
}

func (g GroupIndexes) IndexesList() []collections.Index[keys.GroupPrimaryKey, types.Group] {
	return []collections.Index[keys.GroupPrimaryKey, types.Group]{
		g.State,
		g.Deployment,
	}
}

// NewDeploymentIndexes creates all secondary indexes for the deployment IndexedMap
func NewDeploymentIndexes(sb *collections.SchemaBuilder) DeploymentIndexes {
	return DeploymentIndexes{
		State: indexes.NewMulti(
			sb,
			collections.NewPrefix(keys.DeploymentIndexStatePrefix),
			"deployments_by_state",
			collections.Int32Key,
			keys.DeploymentPrimaryKeyCodec,
			func(_ keys.DeploymentPrimaryKey, deployment v1.Deployment) (int32, error) {
				return int32(deployment.State), nil
			},
		),
	}
}

// NewGroupIndexes creates all secondary indexes for the group IndexedMap
func NewGroupIndexes(sb *collections.SchemaBuilder) GroupIndexes {
	return GroupIndexes{
		State: indexes.NewMulti(
			sb,
			collections.NewPrefix(keys.GroupIndexStatePrefix),
			"groups_by_state",
			collections.Int32Key,
			keys.GroupPrimaryKeyCodec,
			func(_ keys.GroupPrimaryKey, group types.Group) (int32, error) {
				return int32(group.State), nil
			},
		),
		Deployment: indexes.NewMulti(
			sb,
			collections.NewPrefix(keys.GroupIndexDeploymentPrefix),
			"groups_by_deployment",
			keys.DeploymentPrimaryKeyCodec,
			keys.GroupPrimaryKeyCodec,
			func(_ keys.GroupPrimaryKey, group types.Group) (keys.DeploymentPrimaryKey, error) {
				return collections.Join(group.ID.Owner, group.ID.DSeq), nil
			},
		),
	}
}

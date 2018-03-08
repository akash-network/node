package state

import (
	"math"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
)

type DeploymentGroupAdapter interface {
	Save(*types.DeploymentGroup) error
	Get(addr base.Bytes, seq uint64) (*types.DeploymentGroup, error)
	ForDeployment(addr base.Bytes) ([]*types.DeploymentGroup, error)
}

func NewDeploymentGroupAdapter(db DB) DeploymentGroupAdapter {
	return &deploymentGroupAdapter{db}
}

type deploymentGroupAdapter struct {
	db DB
}

func (a *deploymentGroupAdapter) Save(obj *types.DeploymentGroup) error {
	path := a.KeyFor(a.IDFor(obj))
	return saveObject(a.db, path, obj)
}

func (a *deploymentGroupAdapter) Get(addr base.Bytes, seq uint64) (*types.DeploymentGroup, error) {
	path := a.KeyFor(DeploymentGroupID(addr, seq))
	buf := a.db.Get(path)
	obj := new(types.DeploymentGroup)
	return obj, proto.Unmarshal(buf, obj)
}

func (a *deploymentGroupAdapter) ForDeployment(deployment base.Bytes) ([]*types.DeploymentGroup, error) {
	min := a.deploymentMinRange(deployment)
	max := a.deploymentMaxRange(deployment)

	_, bufs, _, err := a.db.GetRangeWithProof(min, max, MaxRangeLimit)
	if err != nil {
		return nil, err
	}

	var items []*types.DeploymentGroup

	for _, buf := range bufs {
		item := &types.DeploymentGroup{}
		if err := item.Unmarshal(buf); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, nil
}

// /deployment-groups/{deployment-address}{group-sequence}
func (a *deploymentGroupAdapter) KeyFor(id []byte) []byte {
	return append([]byte(DeploymentGroupPath), id...)
}

// {deployment-address}{group-sequence}
func (a *deploymentGroupAdapter) IDFor(obj *types.DeploymentGroup) []byte {
	return DeploymentGroupID(obj.Deployment, obj.GetSeq())
}

// /deployment-groups/{deployment-address}{0}
func (a *deploymentGroupAdapter) deploymentMinRange(deployment []byte) []byte {
	return a.KeyFor(DeploymentGroupID(deployment, 0))
}

// /deployment-groups/{deployment-address}{max-uint}
func (a *deploymentGroupAdapter) deploymentMaxRange(deployment []byte) []byte {
	return a.KeyFor(DeploymentGroupID(deployment, math.MaxUint64))
}

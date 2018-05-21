package state

import (
	"math"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
)

type DeploymentGroupAdapter interface {
	Save(*types.DeploymentGroup) error
	Get(id types.DeploymentGroupID) (*types.DeploymentGroup, error)
	ForDeployment(addr base.Bytes) ([]*types.DeploymentGroup, error)
}

func NewDeploymentGroupAdapter(db DB) DeploymentGroupAdapter {
	return &deploymentGroupAdapter{db}
}

type deploymentGroupAdapter struct {
	db DB
}

func (a *deploymentGroupAdapter) Save(obj *types.DeploymentGroup) error {
	path := a.keyFor(obj.DeploymentGroupID)
	return saveObject(a.db, path, obj)
}

func (a *deploymentGroupAdapter) Get(id types.DeploymentGroupID) (*types.DeploymentGroup, error) {
	path := a.keyFor(id)
	buf := a.db.Get(path)
	if buf == nil {
		return nil, nil
	}
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

func (a *deploymentGroupAdapter) keyFor(id types.DeploymentGroupID) base.Bytes {
	key := keys.DeploymentGroupID(id).Bytes()
	return append(base.Bytes(DeploymentGroupPath), key...)
}

func (a *deploymentGroupAdapter) deploymentMinRange(deployment []byte) []byte {
	return a.keyFor(types.DeploymentGroupID{
		Deployment: deployment,
		Seq:        0,
	})
}

func (a *deploymentGroupAdapter) deploymentMaxRange(deployment []byte) []byte {
	return a.keyFor(types.DeploymentGroupID{
		Deployment: deployment,
		Seq:        math.MaxUint64,
	})
}

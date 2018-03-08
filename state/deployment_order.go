package state

import (
	"math"

	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
)

type DeploymentOrderAdapter interface {
	Save(*types.DeploymentOrder) error
	GetByKey(base.Bytes) (*types.DeploymentOrder, error)
	Get(addr base.Bytes, group uint64, order uint64) (*types.DeploymentOrder, error)
	ForGroup(*types.DeploymentGroup) ([]*types.DeploymentOrder, error)
	All() ([]*types.DeploymentOrder, error)
	KeyFor(base.Bytes) base.Bytes
}

func NewDeploymentOrderAdapter(db DB) DeploymentOrderAdapter {
	return &deploymentOrderAdapter{db}
}

type deploymentOrderAdapter struct {
	db DB
}

func (a *deploymentOrderAdapter) Save(obj *types.DeploymentOrder) error {
	path := a.KeyFor(a.IDFor(obj))
	return saveObject(a.db, path, obj)
}

func (d *deploymentOrderAdapter) Get(addr base.Bytes, group uint64, order uint64) (*types.DeploymentOrder, error) {
	return d.GetByKey(DeploymentOrderID(addr, group, order))
}

func (d *deploymentOrderAdapter) GetByKey(address base.Bytes) (*types.DeploymentOrder, error) {
	depo := types.DeploymentOrder{}
	key := d.KeyFor(address)
	buf := d.db.Get(key)
	if buf == nil {
		return nil, nil
	}
	return &depo, depo.Unmarshal(buf)
}

func (a *deploymentOrderAdapter) ForGroup(group *types.DeploymentGroup) ([]*types.DeploymentOrder, error) {
	min := a.groupMinRange(group)
	max := a.groupMaxRange(group)
	return a.forRange(min, max)
}

func (a *deploymentOrderAdapter) All() ([]*types.DeploymentOrder, error) {
	min := a.allMinRange()
	max := a.allMaxRange()
	return a.forRange(min, max)
}

// /deployment-orders/{deployment-address}{group-sequence}{order-sequence}
func (a *deploymentOrderAdapter) KeyFor(id base.Bytes) base.Bytes {
	return append([]byte(DeploymentOrderPath), id...)
}

// {deployment-address}{group-sequence}{order-sequence}{provider-address}
func (a *deploymentOrderAdapter) IDFor(obj *types.DeploymentOrder) []byte {
	return DeploymentOrderID(obj.Deployment, obj.GetGroup(), obj.GetOrder())
}

// /deployment-orders/{deployment-address}{group-sequence}{order-sequence}
func (a *deploymentOrderAdapter) groupMinRange(group *types.DeploymentGroup) []byte {
	return a.KeyFor(DeploymentOrderID(group.Deployment, group.GetSeq(), 0))
}

// /deployment-orders/{deployment-address}{group-sequence}{max-order-sequence}
func (a *deploymentOrderAdapter) groupMaxRange(group *types.DeploymentGroup) []byte {
	return a.KeyFor(DeploymentOrderID(group.Deployment, group.GetSeq(), math.MaxUint64))
}

// /deployment-orders/{min-address}{0}{0}
func (a *deploymentOrderAdapter) allMinRange() []byte {
	return a.KeyFor(DeploymentOrderID(MinAddress(), 0, 0))
}

// /deployment-orders/{max-address}{max-group-sequence}{max-order-sequence}
func (a *deploymentOrderAdapter) allMaxRange() []byte {
	return a.KeyFor(DeploymentOrderID(MaxAddress(), math.MaxUint64, math.MaxUint64))
}

func (a *deploymentOrderAdapter) forRange(min, max []byte) ([]*types.DeploymentOrder, error) {
	_, bufs, _, err := a.db.GetRangeWithProof(min, max, MaxRangeLimit)
	if err != nil {
		return nil, err
	}

	var items []*types.DeploymentOrder

	for _, buf := range bufs {
		item := &types.DeploymentOrder{}
		if err := item.Unmarshal(buf); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

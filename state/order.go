package state

import (
	"math"

	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
)

type OrderAdapter interface {
	Save(*types.Order) error
	Get(types.OrderID) (*types.Order, error)
	ForDeployment(base.Bytes) ([]*types.Order, error)
	ForGroup(types.DeploymentGroupID) ([]*types.Order, error)
	All() ([]*types.Order, error)
}

func NewOrderAdapter(db DB) OrderAdapter {
	return &orderAdapter{db}
}

type orderAdapter struct {
	db DB
}

func (a *orderAdapter) Save(obj *types.Order) error {
	path := a.keyFor(obj.OrderID)
	return saveObject(a.db, path, obj)
}

func (d *orderAdapter) Get(id types.OrderID) (*types.Order, error) {
	key := d.keyFor(id)
	depo := types.Order{}
	buf := d.db.Get(key)
	if buf == nil {
		return nil, nil
	}
	return &depo, depo.Unmarshal(buf)
}

func (a *orderAdapter) ForDeployment(deployment base.Bytes) ([]*types.Order, error) {
	min := a.deploymentMinRange(deployment)
	max := a.deploymentMaxRange(deployment)
	return a.forRange(min, max)
}

func (a *orderAdapter) ForGroup(id types.DeploymentGroupID) ([]*types.Order, error) {
	min := a.groupMinRange(id)
	max := a.groupMaxRange(id)
	return a.forRange(min, max)
}

func (a *orderAdapter) All() ([]*types.Order, error) {
	min := a.allMinRange()
	max := a.allMaxRange()
	return a.forRange(min, max)
}

func (a *orderAdapter) keyFor(id types.OrderID) base.Bytes {
	path := keys.OrderID(id).Bytes()
	return append([]byte(OrderPath), path...)
}

func (a *orderAdapter) deploymentMinRange(deployment base.Bytes) []byte {
	return a.keyFor(types.OrderID{
		Deployment: deployment,
	})
}

func (a *orderAdapter) deploymentMaxRange(deployment base.Bytes) []byte {
	return a.keyFor(types.OrderID{
		Deployment: deployment,
		Group:      math.MaxUint64,
		Seq:        math.MaxUint64,
	})
}

func (a *orderAdapter) groupMinRange(id types.DeploymentGroupID) []byte {
	return a.keyFor(types.OrderID{
		Deployment: id.Deployment,
		Group:      id.Seq,
	})
}

func (a *orderAdapter) groupMaxRange(id types.DeploymentGroupID) []byte {
	return a.keyFor(types.OrderID{
		Deployment: id.Deployment,
		Group:      id.Seq,
		Seq:        math.MaxUint64,
	})
}

func (a *orderAdapter) allMinRange() []byte {
	return a.keyFor(types.OrderID{
		Deployment: MinAddress(),
	})
}

func (a *orderAdapter) allMaxRange() []byte {
	return a.keyFor(types.OrderID{
		Deployment: MaxAddress(),
		Group:      math.MaxUint64,
		Seq:        math.MaxUint64,
	})
}

func (a *orderAdapter) forRange(min, max []byte) ([]*types.Order, error) {
	_, bufs, _, err := a.db.GetRangeWithProof(min, max, MaxRangeLimit)
	if err != nil {
		return nil, err
	}

	var items []*types.Order

	for _, buf := range bufs {
		item := &types.Order{}
		if err := item.Unmarshal(buf); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

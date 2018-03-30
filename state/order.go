package state

import (
	"math"

	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
)

type OrderAdapter interface {
	Save(*types.Order) error
	GetByKey(base.Bytes) (*types.Order, error)
	Get(addr base.Bytes, group uint64, order uint64) (*types.Order, error)
	ForDeployment(base.Bytes) ([]*types.Order, error)
	ForGroup(*types.DeploymentGroup) ([]*types.Order, error)
	All() ([]*types.Order, error)
	KeyFor(base.Bytes) base.Bytes
}

func NewOrderAdapter(db DB) OrderAdapter {
	return &orderAdapter{db}
}

type orderAdapter struct {
	db DB
}

func (a *orderAdapter) Save(obj *types.Order) error {
	path := a.KeyFor(a.IDFor(obj))
	return saveObject(a.db, path, obj)
}

func (d *orderAdapter) Get(addr base.Bytes, group uint64, order uint64) (*types.Order, error) {
	return d.GetByKey(OrderID(addr, group, order))
}

func (d *orderAdapter) GetByKey(address base.Bytes) (*types.Order, error) {
	depo := types.Order{}
	key := d.KeyFor(address)
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

func (a *orderAdapter) ForGroup(group *types.DeploymentGroup) ([]*types.Order, error) {
	min := a.groupMinRange(group)
	max := a.groupMaxRange(group)
	return a.forRange(min, max)
}

func (a *orderAdapter) All() ([]*types.Order, error) {
	min := a.allMinRange()
	max := a.allMaxRange()
	return a.forRange(min, max)
}

// /deployment-orders/{deployment-address}{group-sequence}{order-sequence}
func (a *orderAdapter) KeyFor(id base.Bytes) base.Bytes {
	return append([]byte(OrderPath), id...)
}

// {deployment-address}{group-sequence}{order-sequence}{provider-address}
func (a *orderAdapter) IDFor(obj *types.Order) []byte {
	return OrderID(obj.Deployment, obj.GetGroup(), obj.GetOrder())
}

// /deployment-orders/{deployment-address}{group-sequence}{order-sequence}
func (a *orderAdapter) deploymentMinRange(deployment base.Bytes) []byte {
	return a.KeyFor(OrderID(deployment, 0, 0))
}

// /deployment-orders/{deployment-address}{group-sequence}{max-order-sequence}
func (a *orderAdapter) deploymentMaxRange(deployment base.Bytes) []byte {
	return a.KeyFor(OrderID(deployment, math.MaxUint64, math.MaxUint64))
}

// /deployment-orders/{deployment-address}{group-sequence}{order-sequence}
func (a *orderAdapter) groupMinRange(group *types.DeploymentGroup) []byte {
	return a.KeyFor(OrderID(group.Deployment, group.GetSeq(), 0))
}

// /deployment-orders/{deployment-address}{group-sequence}{max-order-sequence}
func (a *orderAdapter) groupMaxRange(group *types.DeploymentGroup) []byte {
	return a.KeyFor(OrderID(group.Deployment, group.GetSeq(), math.MaxUint64))
}

// /deployment-orders/{min-address}{0}{0}
func (a *orderAdapter) allMinRange() []byte {
	return a.KeyFor(OrderID(MinAddress(), 0, 0))
}

// /deployment-orders/{max-address}{max-group-sequence}{max-order-sequence}
func (a *orderAdapter) allMaxRange() []byte {
	return a.KeyFor(OrderID(MaxAddress(), math.MaxUint64, math.MaxUint64))
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

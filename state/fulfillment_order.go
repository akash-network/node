package state

import (
	"math"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
)

type FulfillmentOrderAdapter interface {
	Save(*types.FulfillmentOrder) error
	Get(daddr base.Bytes, group uint64, order uint64, paddr base.Bytes) (*types.FulfillmentOrder, error)
	ForGroup(*types.DeploymentGroup) ([]*types.FulfillmentOrder, error)
	ForDeploymentOrder(*types.DeploymentOrder) ([]*types.FulfillmentOrder, error)
}

func NewFulfillmentOrderAdapter(db DB) FulfillmentOrderAdapter {
	return &fulfillmentOrderAdapter{db}
}

type fulfillmentOrderAdapter struct {
	db DB
}

func (a *fulfillmentOrderAdapter) Save(obj *types.FulfillmentOrder) error {
	path := a.KeyFor(a.IDFor(obj))
	return saveObject(a.db, path, obj)
}

func (a *fulfillmentOrderAdapter) Get(daddr base.Bytes, group uint64, order uint64, paddr base.Bytes) (*types.FulfillmentOrder, error) {
	path := a.KeyFor(FulfillmentOrderID(daddr, group, order, paddr))
	buf := a.db.Get(path)
	if buf == nil {
		return nil, nil
	}
	obj := new(types.FulfillmentOrder)
	return obj, proto.Unmarshal(buf, obj)
}

func (a *fulfillmentOrderAdapter) ForGroup(group *types.DeploymentGroup) ([]*types.FulfillmentOrder, error) {
	min := a.groupMinRange(group)
	max := a.groupMaxRange(group)
	return a.forRange(min, max)
}

func (a *fulfillmentOrderAdapter) ForDeploymentOrder(order *types.DeploymentOrder) ([]*types.FulfillmentOrder, error) {
	min := a.orderMinRange(order)
	max := a.orderMaxRange(order)
	return a.forRange(min, max)
}

// /fulfillment-orders/{deployment-address}{group-sequence}{order-sequence}{provider-address}
func (a *fulfillmentOrderAdapter) KeyFor(id []byte) []byte {
	return append([]byte(FulfillmentOrderPath), id...)
}

// {deployment-address}{group-sequence}{order-sequence}{provider-address}
func (a *fulfillmentOrderAdapter) IDFor(obj *types.FulfillmentOrder) []byte {
	return FulfillmentOrderID(obj.Deployment, obj.GetGroup(), obj.GetOrder(), obj.Provider)
}

// /fulfillment-orders/{deployment-address}{group-sequence}{order-sequence}
func (a *fulfillmentOrderAdapter) groupMinRange(group *types.DeploymentGroup) []byte {
	return a.KeyFor(FulfillmentOrderID(group.Deployment, group.GetSeq(), 0, []byte{}))
}

// /fulfillment-orders/{deployment-address}{group-sequence}{max-order-sequence}{max-address}
func (a *fulfillmentOrderAdapter) groupMaxRange(group *types.DeploymentGroup) []byte {
	return a.KeyFor(FulfillmentOrderID(group.Deployment, group.GetSeq(), math.MaxUint64, MaxAddress()))
}

// /fulfillment-orders/{deployment-address}{group-sequence}{order-sequence}
func (a *fulfillmentOrderAdapter) orderMinRange(order *types.DeploymentOrder) []byte {
	return a.KeyFor(FulfillmentOrderID(order.Deployment, order.GetGroup(), order.GetOrder(), []byte{}))
}

// /fulfillment-orders/{deployment-address}{group-sequence}{max-order-sequence}{max-address}
func (a *fulfillmentOrderAdapter) orderMaxRange(order *types.DeploymentOrder) []byte {
	return a.KeyFor(FulfillmentOrderID(order.Deployment, order.GetGroup(), order.GetOrder(), MaxAddress()))
}

func (a *fulfillmentOrderAdapter) forRange(min, max []byte) ([]*types.FulfillmentOrder, error) {
	_, bufs, _, err := a.db.GetRangeWithProof(min, max, MaxRangeLimit)
	if err != nil {
		return nil, err
	}

	var items []*types.FulfillmentOrder

	for _, buf := range bufs {
		item := &types.FulfillmentOrder{}
		if err := item.Unmarshal(buf); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
